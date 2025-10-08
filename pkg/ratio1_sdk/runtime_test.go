package ratio1_sdk_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/ratio1_sdk"
)

func TestNewFromEnvHTTPMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_status":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"keys":[]}`))
		case "/add_file_base64":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"cid":"test-cid"}`))
		case "/get_file_base64":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"file_base64_str":"","filename":"empty"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	t.Setenv("R1_RUNTIME_MODE", "http")
	t.Setenv("EE_CHAINSTORE_API_URL", srv.URL)
	t.Setenv("EE_R1FS_API_URL", srv.URL)

	cs, fs, mode, err := ratio1_sdk.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "http" {
		t.Fatalf("expected http mode, got %q", mode)
	}
	if cs == nil || fs == nil {
		t.Fatalf("expected non-nil clients")
	}
}

func TestNewFromEnvMockAutoFallback(t *testing.T) {
	t.Setenv("R1_RUNTIME_MODE", "")
	t.Setenv("EE_CHAINSTORE_API_URL", "")
	t.Setenv("EE_R1FS_API_URL", "")

	cs, _, mode, err := ratio1_sdk.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	ctx := context.Background()
	if _, err := cstore.Put(ctx, cs, "key", map[string]int{"count": 1}, nil); err != nil {
		t.Fatalf("mock Put: %v", err)
	}
}

func TestNewFromEnvSeeds(t *testing.T) {
	cstoreSeed := `[{"key":"foo","value":{"answer":42}}]`
	cstoreFile := writeTempFile(t, "cstore-seed.json", []byte(cstoreSeed))
	defer os.Remove(cstoreFile)

	r1fsSeed := `[{"path":"/seed.txt","base64":"` + base64.StdEncoding.EncodeToString([]byte("seed-data")) + `","content_type":"text/plain"}]`
	r1fsFile := writeTempFile(t, "r1fs-seed.json", []byte(r1fsSeed))
	defer os.Remove(r1fsFile)

	t.Setenv("R1_RUNTIME_MODE", "mock")
	t.Setenv("R1_MOCK_CSTORE_SEED", cstoreFile)
	t.Setenv("R1_MOCK_R1FS_SEED", r1fsFile)

	cs, fs, mode, err := ratio1_sdk.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	ctx := context.Background()
	item, err := cstore.Get[map[string]int](ctx, cs, "foo")
	if err != nil {
		t.Fatalf("Get seeded value: %v", err)
	}
	if item == nil || item.Value["answer"] != 42 {
		t.Fatalf("unexpected seeded item: %#v", item)
	}

	var buf bytes.Buffer
	n, err := fs.Download(ctx, "/seed.txt", &buf)
	if err != nil {
		t.Fatalf("Download seeded file: %v", err)
	}
	if n == 0 || buf.String() != "seed-data" {
		t.Fatalf("unexpected seeded file contents: %q", buf.String())
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
