package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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
	if err := client.Set(ctx, "jobs:1", counter{Count: 1}, nil); err != nil {
		panic(err)
	}
	var itemValue counter
	if _, err := client.Get(ctx, "jobs:1", &itemValue); err != nil {
		panic(err)
	}
	fmt.Printf("jobs:1 => %+v\n", itemValue)

	fmt.Println("\n== GetStatus ==")
	status, err := client.GetStatus(ctx)
	if err != nil {
		panic(err)
	}
	if status != nil {
		fmt.Printf("keys => %v\n", status.Keys)
	}

	fmt.Println("\n== Hash operations (HSet/HGet/HGetAll) ==")
	if err := client.HSet(ctx, "h:jobs", "1", map[string]string{"status": "queued"}, nil); err != nil {
		panic(err)
	}
	var hValue map[string]string
	if _, err := client.HGet(ctx, "h:jobs", "1", &hValue); err != nil {
		panic(err)
	}
	fmt.Println("field 1 ->", hValue)
	if err := client.HSet(ctx, "h:jobs", "2", map[string]string{"status": "running"}, nil); err != nil {
		panic(err)
	}
	hItems, err := client.HGetAll(ctx, "h:jobs")
	if err != nil {
		panic(err)
	}
	for _, it := range hItems {
		var value map[string]string
		if err := json.Unmarshal(it.Value, &value); err != nil {
			panic(err)
		}
		fmt.Printf("hash %s field %s -> %v\n", it.HashKey, it.Field, value)
	}
}

func newCStoreServer() *httptest.Server {
	var (
		mu    sync.Mutex
		store = map[string]json.RawMessage{}
		hash  = map[string]map[string]json.RawMessage{}
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			var payload struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			store[payload.Key] = append([]byte(nil), payload.Value...)
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
				Result json.RawMessage `json:"result"`
			}{Result: value}
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
				HashKey string          `json:"hkey"`
				Field   string          `json:"key"`
				Value   json.RawMessage `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			bucket := hash[req.HashKey]
			if bucket == nil {
				bucket = make(map[string]json.RawMessage)
				hash[req.HashKey] = bucket
			}
			bucket[req.Field] = append([]byte(nil), req.Value...)
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
				Result json.RawMessage `json:"result"`
			}{Result: value}
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
				payload[field] = val
			}
			encoded, _ := json.Marshal(payload)
			resp := struct {
				Result json.RawMessage `json:"result"`
			}{Result: json.RawMessage(encoded)}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
}
