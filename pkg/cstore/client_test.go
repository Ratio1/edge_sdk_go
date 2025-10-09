package cstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
)

type counter struct {
	Count int `json:"count"`
}

func TestClientPutGetAndList(t *testing.T) {
	store := map[string]string{}
	hashStore := map[string]map[string]string{}
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			defer r.Body.Close()
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
			io.WriteString(w, "true")
		case "/get":
			key := r.URL.Query().Get("key")
			mu.Lock()
			value, ok := store[key]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				io.WriteString(w, "null")
				return
			}
			result := struct {
				Result string `json:"result"`
			}{Result: strconv.Quote(value)}
			json.NewEncoder(w).Encode(result)
		case "/hset":
			defer r.Body.Close()
			var payload struct {
				HashKey string `json:"hkey"`
				Key     string `json:"key"`
				Value   string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			bucket := hashStore[payload.HashKey]
			if bucket == nil {
				bucket = make(map[string]string)
				hashStore[payload.HashKey] = bucket
			}
			bucket[payload.Key] = payload.Value
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, "true")
		case "/hget":
			hkey := r.URL.Query().Get("hkey")
			key := r.URL.Query().Get("key")
			mu.Lock()
			bucket := hashStore[hkey]
			var (
				value string
				ok    bool
			)
			if bucket != nil {
				value, ok = bucket[key]
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				io.WriteString(w, "null")
				return
			}
			result := struct {
				Result string `json:"result"`
			}{Result: strconv.Quote(value)}
			json.NewEncoder(w).Encode(result)
		case "/hgetall":
			hkey := r.URL.Query().Get("hkey")
			mu.Lock()
			bucket := hashStore[hkey]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if bucket == nil || len(bucket) == 0 {
				io.WriteString(w, "null")
				return
			}
			fields := make(map[string]json.RawMessage, len(bucket))
			for k, v := range bucket {
				fields[k] = json.RawMessage([]byte(v))
			}
			raw, _ := json.Marshal(fields)
			result := struct {
				Result string `json:"result"`
			}{Result: string(raw)}
			json.NewEncoder(w).Encode(result)
		case "/get_status":
			mu.Lock()
			keys := make([]string, 0, len(store))
			for k := range store {
				keys = append(keys, k)
			}
			mu.Unlock()
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/json")
			result := struct {
				Result struct {
					Keys []string `json:"keys"`
				} `json:"result"`
			}{}
			result.Result.Keys = keys
			json.NewEncoder(w).Encode(result)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := cstore.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := cstore.Put(ctx, client, "jobs:123", counter{Count: 1}, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := cstore.Put(ctx, client, "jobs:124", counter{Count: 2}, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := cstore.HSet(ctx, client, "jobs", "123", counter{Count: 3}, nil); err != nil {
		t.Fatalf("HSet: %v", err)
	}

	item, err := cstore.Get[counter](ctx, client, "jobs:123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil || item.Value.Count != 1 {
		t.Fatalf("Get returned unexpected item: %#v", item)
	}

	missing, err := cstore.Get[counter](ctx, client, "missing")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing key, got %#v", missing)
	}

	hItem, err := cstore.HGet[counter](ctx, client, "jobs", "123")
	if err != nil {
		t.Fatalf("HGet: %v", err)
	}
	if hItem == nil || hItem.Value.Count != 3 || hItem.HashKey != "jobs" || hItem.Field != "123" {
		t.Fatalf("HGet returned unexpected item: %#v", hItem)
	}
	hMissing, err := cstore.HGet[counter](ctx, client, "jobs", "999")
	if err != nil {
		t.Fatalf("HGet missing: %v", err)
	}
	if hMissing != nil {
		t.Fatalf("expected nil for missing hash field, got %#v", hMissing)
	}

	all, err := cstore.HGetAll[counter](ctx, client, "jobs")
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	if len(all) != 1 || all[0].Value.Count != 3 || all[0].Field != "123" {
		t.Fatalf("HGetAll returned unexpected items: %#v", all)
	}
	emptyAll, err := cstore.HGetAll[counter](ctx, client, "missing-hash")
	if err != nil {
		t.Fatalf("HGetAll missing: %v", err)
	}
	if emptyAll != nil {
		t.Fatalf("expected nil for missing hash, got %#v", emptyAll)
	}

	result, err := cstore.List[counter](ctx, client, "jobs:", "", 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Value.Count != 1 {
		t.Fatalf("List page 1 mismatch: %#v", result)
	}
	if result.NextCursor == "" {
		t.Fatalf("expected next cursor")
	}

	result2, err := cstore.List[counter](ctx, client, "jobs:", result.NextCursor, 1)
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(result2.Items) != 1 || result2.Items[0].Value.Count != 2 {
		t.Fatalf("List page 2 mismatch: %#v", result2)
	}
	if result2.NextCursor != "" {
		t.Fatalf("expected no more pages, got %q", result2.NextCursor)
	}
}

func TestPutOptionsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected call", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := cstore.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	ttl := 60
	if _, err := cstore.Put(ctx, client, "key", counter{Count: 1}, &cstore.PutOptions{TTLSeconds: &ttl}); !errors.Is(err, cstore.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for TTL, got %v", err)
	}

	if _, err := cstore.Put(ctx, client, "key", counter{Count: 1}, &cstore.PutOptions{IfAbsent: true}); !errors.Is(err, cstore.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for IfAbsent, got %v", err)
	}
}

func TestDeleteUnsupported(t *testing.T) {
	client := &cstore.Client{}
	err := client.Delete(context.Background(), "foo")
	if !errors.Is(err, cstore.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature, got %v", err)
	}
}
