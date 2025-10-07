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
			// The upstream FastAPI implementation returns the stored string.
			io.WriteString(w, strconv.Quote(value))
		case "/get_status":
			mu.Lock()
			keys := make([]string, 0, len(store))
			for k := range store {
				keys = append(keys, k)
			}
			mu.Unlock()
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"keys": keys})
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
