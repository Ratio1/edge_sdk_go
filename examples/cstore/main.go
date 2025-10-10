package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
)

type counter struct {
	Count int `json:"count"`
}

func main() {
	server := newCStoreServer()
	defer server.Close()

	client, err := cstore.New(server.URL)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	fmt.Println("== Put/Get ==")
	if _, err := cstore.Put(ctx, client, "jobs:1", counter{Count: 1}, nil); err != nil {
		panic(err)
	}
	item, err := cstore.Get[counter](ctx, client, "jobs:1")
	if err != nil {
		panic(err)
	}
	fmt.Printf("jobs:1 => %+v\n", item.Value)

	fmt.Println("\n== PutJSON/GetJSON ==")
	if _, err := client.PutJSON(ctx, "jobs:meta", `{"owner":"alice"}`, nil); err != nil {
		panic(err)
	}
	jsonBytes, err := client.GetJSON(ctx, "jobs:meta")
	if err != nil {
		panic(err)
	}
	fmt.Printf("jobs:meta raw JSON: %s\n", string(jsonBytes))

	fmt.Println("\n== List with pagination ==")
	page, err := cstore.List[counter](ctx, client, "jobs:", "", 1)
	if err != nil {
		panic(err)
	}
	for _, it := range page.Items {
		fmt.Printf("page1 -> %s: %+v\n", it.Key, it.Value)
	}
	if page.NextCursor != "" {
		page2, err := cstore.List[counter](ctx, client, "jobs:", page.NextCursor, 1)
		if err != nil {
			panic(err)
		}
		for _, it := range page2.Items {
			fmt.Printf("page2 -> %s: %+v\n", it.Key, it.Value)
		}
	}

	fmt.Println("\n== Hash operations (HSet/HGet/HGetAll) ==")
	if _, err := cstore.HSet(ctx, client, "h:jobs", "1", map[string]string{"status": "queued"}, nil); err != nil {
		panic(err)
	}
	hItem, err := cstore.HGet[map[string]string](ctx, client, "h:jobs", "1")
	if err != nil {
		panic(err)
	}
	fmt.Println("field 1 ->", hItem.Value)
	if _, err := cstore.HSet(ctx, client, "h:jobs", "2", map[string]string{"status": "running"}, nil); err != nil {
		panic(err)
	}
	hItems, err := cstore.HGetAll[map[string]string](ctx, client, "h:jobs")
	if err != nil {
		panic(err)
	}
	for _, it := range hItems {
		fmt.Printf("hash %s field %s -> %v\n", it.HashKey, it.Field, it.Value)
	}
}

func newCStoreServer() *httptest.Server {
	var (
		mu    sync.Mutex
		store = map[string]string{}
		hash  = map[string]map[string]string{}
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			var payload struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			store[payload.Key] = payload.Value
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("true"))

		case "/get":
			key := r.URL.Query().Get("key")
			mu.Lock()
			value, ok := store[key]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				_, _ = w.Write([]byte("null"))
				return
			}
			// Upstream wraps the JSON payload inside result
			resp := struct {
				Result string `json:"result"`
			}{Result: strconv.Quote(value)}
			_ = json.NewEncoder(w).Encode(resp)

		case "/get_status":
			mu.Lock()
			keys := make([]string, 0, len(store))
			for k := range store {
				keys = append(keys, k)
			}
			mu.Unlock()
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"keys": keys},
			})

		case "/hset":
			var req struct {
				HashKey string `json:"hkey"`
				Field   string `json:"key"`
				Value   string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			bucket := hash[req.HashKey]
			if bucket == nil {
				bucket = make(map[string]string)
				hash[req.HashKey] = bucket
			}
			bucket[req.Field] = req.Value
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("true"))

		case "/hget":
			hkey := r.URL.Query().Get("hkey")
			field := r.URL.Query().Get("key")
			mu.Lock()
			bucket := hash[hkey]
			value, ok := bucket[field]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				_, _ = w.Write([]byte("null"))
				return
			}
			resp := struct {
				Result string `json:"result"`
			}{Result: strconv.Quote(value)}
			_ = json.NewEncoder(w).Encode(resp)

		case "/hgetall":
			hkey := r.URL.Query().Get("hkey")
			mu.Lock()
			bucket := hash[hkey]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if len(bucket) == 0 {
				_, _ = w.Write([]byte("null"))
				return
			}
			payload := make(map[string]json.RawMessage, len(bucket))
			for field, val := range bucket {
				payload[field] = json.RawMessage([]byte(val))
			}
			encoded, _ := json.Marshal(payload)
			resp := struct {
				Result string `json:"result"`
			}{Result: string(encoded)}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
}
