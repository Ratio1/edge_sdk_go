package cstore_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Ratio1/edge_sdk_go/pkg/cstore"
)

func TestNewFromEnvHTTP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
	srv := newLocalHTTPServer(t, handler)
	defer srv.Close()

	t.Setenv("EE_CHAINSTORE_API_URL", srv.URL)

	client, err := cstore.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}

	item, err := client.Get(context.Background(), "missing", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item != nil {
		t.Fatalf("expected nil item, got %#v", item)
	}
}

func TestNewFromEnvMissingURL(t *testing.T) {
	t.Setenv("EE_CHAINSTORE_API_URL", "")

	if _, err := cstore.NewFromEnv(); err == nil {
		t.Fatalf("expected error for unset URL")
	}
}

func TestNewFromEnvInvalidURL(t *testing.T) {
	t.Setenv("EE_CHAINSTORE_API_URL", "://not-a-url")

	if _, err := cstore.NewFromEnv(); err == nil {
		t.Fatalf("expected error for invalid URL")
	}
}
