package cstore_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
)

func TestNewFromEnvHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("null"))
		case "/get_status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"keys": []string{}},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	t.Setenv("R1_RUNTIME_MODE", "http")
	t.Setenv("EE_CHAINSTORE_API_URL", srv.URL)

	client, mode, err := cstore.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "http" {
		t.Fatalf("expected http mode, got %q", mode)
	}

	item, err := client.Get(context.Background(), "missing", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item != nil {
		t.Fatalf("expected nil item, got %#v", item)
	}
}

func TestNewFromEnvMockFallback(t *testing.T) {
	t.Setenv("R1_RUNTIME_MODE", "auto")
	t.Setenv("EE_CHAINSTORE_API_URL", "")

	client, mode, err := cstore.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	ctx := context.Background()
	if _, err := client.Set(ctx, "key", map[string]int{"count": 1}, nil); err != nil {
		t.Fatalf("mock Set: %v", err)
	}
}

func TestNewFromEnvSeed(t *testing.T) {
	seed := `[{"key":"foo","value":{"answer":42}}]`
	file := writeTempFile(t, "cstore-seed.json", []byte(seed))

	t.Setenv("R1_RUNTIME_MODE", "mock")
	t.Setenv("R1_MOCK_CSTORE_SEED", file)

	client, mode, err := cstore.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	var seeded map[string]int
	item, err := client.Get(context.Background(), "foo", &seeded)
	if err != nil {
		t.Fatalf("Get seeded value: %v", err)
	}
	if item == nil || seeded["answer"] != 42 {
		t.Fatalf("unexpected seeded item: %#v value=%#v", item, seeded)
	}
}

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
