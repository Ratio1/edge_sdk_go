package cstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
)

type counter struct {
	Count int `json:"count"`
}

func TestClientSetGetAndStatus(t *testing.T) {
	srv := newTestCStoreServer()
	defer srv.Close()

	client, err := cstore.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := client.Set(ctx, "jobs:123", counter{Count: 1}, nil); err != nil {
		t.Fatalf("Set jobs:123: %v", err)
	}
	if _, err := client.Set(ctx, "jobs:124", counter{Count: 2}, nil); err != nil {
		t.Fatalf("Set jobs:124: %v", err)
	}
	if _, err := client.HSet(ctx, "jobs", "123", counter{Count: 3}, nil); err != nil {
		t.Fatalf("HSet: %v", err)
	}

	var itemValue counter
	item, err := client.Get(ctx, "jobs:123", &itemValue)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil || itemValue.Count != 1 {
		t.Fatalf("Get returned unexpected item: %#v value=%#v", item, itemValue)
	}

	missing, err := client.Get(ctx, "missing", nil)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing key, got %#v", missing)
	}

	var hashValue counter
	hashItem, err := client.HGet(ctx, "jobs", "123", &hashValue)
	if err != nil {
		t.Fatalf("HGet: %v", err)
	}
	if hashItem == nil || hashValue.Count != 3 {
		t.Fatalf("HGet returned unexpected item: %#v value=%#v", hashItem, hashValue)
	}

	hashMissing, err := client.HGet(ctx, "jobs", "999", nil)
	if err != nil {
		t.Fatalf("HGet missing: %v", err)
	}
	if hashMissing != nil {
		t.Fatalf("expected nil for missing hash field, got %#v", hashMissing)
	}

	all, err := client.HGetAll(ctx, "jobs")
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	if len(all) != 1 || all[0].Field != "123" {
		t.Fatalf("HGetAll returned unexpected items: %#v", all)
	}
	var decoded counter
	if err := json.Unmarshal(all[0].Value, &decoded); err != nil {
		t.Fatalf("HGetAll decode: %v", err)
	}
	if decoded.Count != 3 {
		t.Fatalf("HGetAll decoded unexpected value: %#v", decoded)
	}

	status, err := client.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status == nil {
		t.Fatalf("expected non-nil status")
	}
	wantKeys := []string{"jobs:123", "jobs:124"}
	if len(status.Keys) != len(wantKeys) {
		t.Fatalf("GetStatus keys mismatch: %#v", status.Keys)
	}
	for i, key := range wantKeys {
		if status.Keys[i] != key {
			t.Fatalf("GetStatus keys mismatch: got %v want %v", status.Keys, wantKeys)
		}
	}
}

func TestClientPrimitiveRoundTrips(t *testing.T) {
	srv := newTestCStoreServer()
	defer srv.Close()

	client, err := cstore.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	cases := []struct {
		name     string
		value    any
		wantJSON string
	}{
		{name: "string", value: "alpha", wantJSON: `"alpha"`},
		{name: "int", value: 42, wantJSON: "42"},
		{name: "bool", value: true, wantJSON: "true"},
		{name: "raw", value: json.RawMessage(`{"nested":1}`), wantJSON: `{"nested":1}`},
	}

	for _, tc := range cases {
		key := "primitive:" + tc.name
		if _, err := client.Set(ctx, key, tc.value, nil); err != nil {
			t.Fatalf("Set %s: %v", tc.name, err)
		}

		switch v := tc.value.(type) {
		case string:
			var out string
			item, err := client.Get(ctx, key, &out)
			if err != nil {
				t.Fatalf("Get %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("Get %s mismatch: item=%#v out=%q want=%q", tc.name, item, out, v)
			}
		case int:
			var out int
			item, err := client.Get(ctx, key, &out)
			if err != nil {
				t.Fatalf("Get %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("Get %s mismatch: item=%#v out=%d want=%d", tc.name, item, out, v)
			}
		case bool:
			var out bool
			item, err := client.Get(ctx, key, &out)
			if err != nil {
				t.Fatalf("Get %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("Get %s mismatch: item=%#v out=%v want=%v", tc.name, item, out, v)
			}
		case json.RawMessage:
			var out json.RawMessage
			item, err := client.Get(ctx, key, &out)
			if err != nil {
				t.Fatalf("Get %s: %v", tc.name, err)
			}
			if item == nil || string(out) != string(v) || string(item.Value) != tc.wantJSON {
				t.Fatalf("Get %s mismatch: item=%#v out=%s want=%s", tc.name, item, string(out), string(v))
			}
		default:
			t.Fatalf("unsupported test value type %T", tc.value)
		}

		rawOnly, err := client.Get(ctx, key, nil)
		if err != nil {
			t.Fatalf("Get raw %s: %v", tc.name, err)
		}
		if rawOnly == nil || string(rawOnly.Value) != tc.wantJSON {
			t.Fatalf("Get raw %s mismatch: item=%#v wantJSON=%s", tc.name, rawOnly, tc.wantJSON)
		}

		hashKey := "hash:" + tc.name
		if _, err := client.HSet(ctx, hashKey, "field", tc.value, nil); err != nil {
			t.Fatalf("HSet %s: %v", tc.name, err)
		}

		switch v := tc.value.(type) {
		case string:
			var out string
			item, err := client.HGet(ctx, hashKey, "field", &out)
			if err != nil {
				t.Fatalf("HGet %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("HGet %s mismatch: item=%#v out=%q want=%q", tc.name, item, out, v)
			}
		case int:
			var out int
			item, err := client.HGet(ctx, hashKey, "field", &out)
			if err != nil {
				t.Fatalf("HGet %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("HGet %s mismatch: item=%#v out=%d want=%d", tc.name, item, out, v)
			}
		case bool:
			var out bool
			item, err := client.HGet(ctx, hashKey, "field", &out)
			if err != nil {
				t.Fatalf("HGet %s: %v", tc.name, err)
			}
			if item == nil || out != v || string(item.Value) != tc.wantJSON {
				t.Fatalf("HGet %s mismatch: item=%#v out=%v want=%v", tc.name, item, out, v)
			}
		case json.RawMessage:
			var out json.RawMessage
			item, err := client.HGet(ctx, hashKey, "field", &out)
			if err != nil {
				t.Fatalf("HGet %s: %v", tc.name, err)
			}
			if item == nil || string(out) != string(v) || string(item.Value) != tc.wantJSON {
				t.Fatalf("HGet %s mismatch: item=%#v out=%s want=%s", tc.name, item, string(out), string(v))
			}
		}

		rawHash, err := client.HGet(ctx, hashKey, "field", nil)
		if err != nil {
			t.Fatalf("HGet raw %s: %v", tc.name, err)
		}
		if rawHash == nil || string(rawHash.Value) != tc.wantJSON {
			t.Fatalf("HGet raw %s mismatch: item=%#v wantJSON=%s", tc.name, rawHash, tc.wantJSON)
		}
	}
}

func TestSetOptionsUnsupported(t *testing.T) {
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
	if _, err := client.Set(ctx, "key", counter{Count: 1}, &cstore.SetOptions{TTLSeconds: &ttl}); !errors.Is(err, cstore.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for TTL, got %v", err)
	}

	if _, err := client.Set(ctx, "key", counter{Count: 1}, &cstore.SetOptions{IfAbsent: true}); !errors.Is(err, cstore.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for IfAbsent, got %v", err)
	}
}

func TestClientWriteRejectsFalseResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/set":
			_, _ = w.Write([]byte(`{"result": false}`))
		case "/hset":
			_, _ = w.Write([]byte("false"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := cstore.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := client.Set(context.Background(), "jobs:1", counter{Count: 1}, nil); err == nil {
		t.Fatalf("expected error for rejected set")
	}

	if _, err := client.HSet(context.Background(), "jobs", "1", counter{Count: 2}, nil); err == nil {
		t.Fatalf("expected error for rejected hset")
	}
}

func newTestCStoreServer() *httptest.Server {
	store := map[string]json.RawMessage{}
	hashStore := map[string]map[string]json.RawMessage{}
	var mu sync.Mutex

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			defer r.Body.Close()
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
			result := struct {
				Result json.RawMessage `json:"result"`
			}{Result: value}
			_ = json.NewEncoder(w).Encode(result)
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
			_ = json.NewEncoder(w).Encode(result)
		case "/hset":
			defer r.Body.Close()
			var payload struct {
				HashKey string          `json:"hkey"`
				Key     string          `json:"key"`
				Value   json.RawMessage `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			bucket := hashStore[payload.HashKey]
			if bucket == nil {
				bucket = make(map[string]json.RawMessage)
				hashStore[payload.HashKey] = bucket
			}
			bucket[payload.Key] = append([]byte(nil), payload.Value...)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("true"))
		case "/hget":
			hkey := r.URL.Query().Get("hkey")
			key := r.URL.Query().Get("key")
			mu.Lock()
			bucket := hashStore[hkey]
			var (
				value json.RawMessage
				ok    bool
			)
			if bucket != nil {
				value, ok = bucket[key]
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				_, _ = w.Write([]byte("null"))
				return
			}
			result := struct {
				Result json.RawMessage `json:"result"`
			}{Result: value}
			_ = json.NewEncoder(w).Encode(result)
		case "/hgetall":
			hkey := r.URL.Query().Get("hkey")
			mu.Lock()
			bucket := hashStore[hkey]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if len(bucket) == 0 {
				_, _ = w.Write([]byte("null"))
				return
			}
			fields := make(map[string]json.RawMessage, len(bucket))
			for k, v := range bucket {
				fields[k] = v
			}
			data, err := json.Marshal(fields)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			result := struct {
				Result json.RawMessage `json:"result"`
			}{Result: json.RawMessage(data)}
			_ = json.NewEncoder(w).Encode(result)
		default:
			http.NotFound(w, r)
		}
	}))
}
