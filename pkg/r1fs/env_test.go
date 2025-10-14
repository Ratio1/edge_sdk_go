package r1fs_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func TestNewFromEnvHTTP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get_file_base64":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"file_base64_str": "",
				"filename":        "empty",
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	srv := newLocalHTTPServer(t, handler)
	defer srv.Close()

	t.Setenv("R1_RUNTIME_MODE", "http")
	t.Setenv("EE_R1FS_API_URL", srv.URL)

	client, mode, err := r1fs.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "http" {
		t.Fatalf("expected http mode, got %q", mode)
	}

	if _, _, err := client.GetFileBase64(context.Background(), "/missing", ""); err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
}

func TestNewFromEnvMockFallback(t *testing.T) {
	t.Setenv("R1_RUNTIME_MODE", "auto")
	t.Setenv("EE_R1FS_API_URL", "")

	client, mode, err := r1fs.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	payload := strings.NewReader("hello")
	cid, err := client.AddFileBase64(context.Background(), "hello.txt", payload, int64(payload.Len()), nil)
	if err != nil {
		t.Fatalf("AddFileBase64: %v", err)
	}
	if cid == "" {
		t.Fatalf("AddFileBase64 returned empty cid")
	}
}

func TestNewFromEnvSeed(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("seed-data"))
	seed := `[{"path":"/seed.txt","base64":"` + data + `","content_type":"text/plain"}]`
	file := writeTempFile(t, "r1fs-seed.json", []byte(seed))

	t.Setenv("R1_RUNTIME_MODE", "mock")
	t.Setenv("R1_MOCK_R1FS_SEED", file)

	client, mode, err := r1fs.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if mode != "mock" {
		t.Fatalf("expected mock mode, got %q", mode)
	}

	payload, _, err := client.GetFileBase64(context.Background(), "/seed.txt", "")
	if err != nil {
		t.Fatalf("GetFileBase64 seed: %v", err)
	}
	if len(payload) == 0 || string(payload) != "seed-data" {
		t.Fatalf("unexpected seed contents: %q", string(payload))
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
