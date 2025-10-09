package r1fs_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func TestUploadAndDownload(t *testing.T) {
	var (
		mu        sync.Mutex
		data      = make(map[string][]byte)
		yamlData  = make(map[string]json.RawMessage)
		fileNames = make(map[string]string)
		nextID    = 0
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
			res := struct {
				Result struct {
					Cid string `json:"cid"`
				} `json:"result"`
			}{}
			res.Result.Cid = cid
			json.NewEncoder(w).Encode(res)

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

		case "/add_file":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			payload, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = r.FormValue("body_json")
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-file-%d", nextID)
			data[cid] = payload
			fileNames[cid] = header.Filename
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"message": "ok",
					"cid":     cid,
				},
			})

		case "/get_file":
			cid := r.URL.Query().Get("cid")
			mu.Lock()
			_, ok := data[cid]
			filename := fileNames[cid]
			mu.Unlock()
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			path := "/tmp/" + cid
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"file_path": path,
					"meta": map[string]any{
						"file":     path,
						"filename": filename,
					},
				},
			})

		case "/add_yaml":
			defer r.Body.Close()
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Fn     string          `json:"fn"`
				Secret string          `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload.Data) == 0 {
				http.Error(w, "missing data", http.StatusBadRequest)
				return
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-yaml-%d", nextID)
			yamlData[cid] = append([]byte(nil), payload.Data...)
			if payload.Fn == "" {
				fileNames[cid] = "document.yaml"
			} else {
				fileNames[cid] = payload.Fn
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"cid": cid,
				},
			})

		case "/get_yaml":
			cid := r.URL.Query().Get("cid")
			mu.Lock()
			raw, ok := yamlData[cid]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				json.NewEncoder(w).Encode("error")
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"file_data": json.RawMessage(raw),
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

	streamPayload := strings.NewReader("stream upload")
	streamStat, err := client.AddFile(ctx, "stream.txt", streamPayload, int64(streamPayload.Len()), &r1fs.UploadOptions{Secret: "s3"})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if streamStat == nil || streamStat.Path == "" {
		t.Fatalf("AddFile returned invalid stat: %#v", streamStat)
	}
	loc, err := client.GetFile(ctx, streamStat.Path, "s3")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if loc == nil || loc.Filename != "stream.txt" || loc.Path == "" {
		t.Fatalf("unexpected file location: %#v", loc)
	}

	yamlPayload := map[string]any{"name": "ratio1", "count": 2}
	yamlCID, err := client.AddYAML(ctx, yamlPayload, &r1fs.YAMLOptions{Filename: "config.yaml"})
	if err != nil {
		t.Fatalf("AddYAML: %v", err)
	}
	if strings.TrimSpace(yamlCID) == "" {
		t.Fatalf("AddYAML returned empty cid")
	}
	doc, err := r1fs.GetYAML[map[string]any](ctx, client, yamlCID, "")
	if err != nil {
		t.Fatalf("GetYAML: %v", err)
	}
	if doc == nil || doc.Data["name"] != "ratio1" {
		t.Fatalf("unexpected YAML document: %#v", doc)
	}
	if _, err := r1fs.GetYAML[map[string]any](ctx, client, "missing-yaml", ""); err == nil {
		t.Fatalf("expected error for missing YAML document")
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
