package r1fs_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func TestUploadAndDownload(t *testing.T) {
	var (
		mu     sync.Mutex
		data   = make(map[string][]byte)
		nextID = 0
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add_file_base64":
			defer r.Body.Close()
			var payload struct {
				FileBase64 string `json:"file_base64_str"`
				Filename   string `json:"filename"`
				Secret     string `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			raw, err := base64.StdEncoding.DecodeString(payload.FileBase64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()
			nextID++
			cid := "CID-" + payload.Filename
			data[cid] = raw
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"cid": cid})

		case "/get_file_base64":
			defer r.Body.Close()
			var req struct {
				CID string `json:"cid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			payload, ok := data[req.CID]
			mu.Unlock()
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"file_base64_str": base64.StdEncoding.EncodeToString(payload),
				"filename":        "download.bin",
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := r1fs.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	payload := strings.NewReader("hello world")
	stat, err := client.Upload(ctx, "/tmp/hello.txt", payload, int64(payload.Len()), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if stat == nil || stat.Path == "" {
		t.Fatalf("Upload returned invalid stat: %#v", stat)
	}

	var buf strings.Builder
	n, err := client.Download(ctx, stat.Path, &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if n != int64(len("hello world")) || buf.String() != "hello world" {
		t.Fatalf("Download mismatch: n=%d buf=%q", n, buf.String())
	}
}

func TestUnsupportedOperations(t *testing.T) {
	client, err := r1fs.New("mocked_test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Stat(context.Background(), "cid"); !errors.Is(err, r1fs.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for Stat, got %v", err)
	}
	if _, err := client.List(context.Background(), "/", "", 10); !errors.Is(err, r1fs.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for List, got %v", err)
	}
	if err := client.Delete(context.Background(), "cid"); !errors.Is(err, r1fs.ErrUnsupportedFeature) {
		t.Fatalf("expected ErrUnsupportedFeature for Delete, got %v", err)
	}
}
