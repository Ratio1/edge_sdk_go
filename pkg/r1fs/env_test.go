package r1fs_test

import (
	"context"
	"encoding/json"
	"net/http"
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

	t.Setenv("EE_R1FS_API_URL", srv.URL)

	client, err := r1fs.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}

	if _, _, err := client.GetFileBase64(context.Background(), "/missing", ""); err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
}

func TestNewFromEnvMissingURL(t *testing.T) {
	t.Setenv("EE_R1FS_API_URL", "")

	if _, err := r1fs.NewFromEnv(); err == nil {
		t.Fatalf("expected error for unset URL")
	}
}

func TestNewFromEnvInvalidURL(t *testing.T) {
	t.Setenv("EE_R1FS_API_URL", "://not-a-url")

	if _, err := r1fs.NewFromEnv(); err == nil {
		t.Fatalf("expected error for invalid URL")
	}
}
