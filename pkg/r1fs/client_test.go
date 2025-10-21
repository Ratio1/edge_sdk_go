package r1fs_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Ratio1/edge_sdk_go/pkg/r1fs"
)

func TestAddFileBase64AndGetFileBase64(t *testing.T) {
	var (
		mu        sync.Mutex
		data      = make(map[string][]byte)
		yamlData  = make(map[string]json.RawMessage)
		fileNames = make(map[string]string)
		nextID    = 0
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add_file_base64":
			defer r.Body.Close()
			var payload struct {
				FileBase64 string `json:"file_base64_str"`
				Filename   string `json:"filename"`
				FilePath   string `json:"file_path"`
				Secret     string `json:"secret"`
				Nonce      *int   `json:"nonce"`
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

			name := payload.Filename
			if strings.TrimSpace(name) == "" {
				name = payload.FilePath
			}
			if strings.TrimSpace(name) == "" {
				http.Error(w, "filename required", http.StatusBadRequest)
				return
			}
			mu.Lock()
			nextID++
			cid := "CID-" + name
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
			filename := header.Filename
			metaRaw := r.FormValue("body_json")
			if strings.TrimSpace(metaRaw) != "" {
				var meta map[string]any
				if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if fn, ok := meta["fn"].(string); ok && strings.TrimSpace(fn) != "" {
					filename = fn
				}
				if fp, ok := meta["file_path"].(string); ok && strings.TrimSpace(fp) != "" {
					filename = fp
				}
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-file-%d", nextID)
			data[cid] = payload
			fileNames[cid] = filename
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"message": "ok",
					"cid":     cid,
				},
			})

		case "/add_json":
			defer r.Body.Close()
			var payload struct {
				Data     json.RawMessage `json:"data"`
				Fn       string          `json:"fn"`
				FilePath string          `json:"file_path"`
				Secret   string          `json:"secret"`
				Nonce    *int            `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload.Data) == 0 {
				http.Error(w, "missing data", http.StatusBadRequest)
				return
			}
			filename := payload.Fn
			if strings.TrimSpace(filename) == "" {
				filename = payload.FilePath
			}
			if strings.TrimSpace(filename) == "" {
				filename = "data.json"
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-json-%d", nextID)
			data[cid] = append([]byte(nil), payload.Data...)
			fileNames[cid] = filename
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/add_pickle":
			defer r.Body.Close()
			var payload struct {
				Data     json.RawMessage `json:"data"`
				Fn       string          `json:"fn"`
				FilePath string          `json:"file_path"`
				Secret   string          `json:"secret"`
				Nonce    *int            `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload.Data) == 0 {
				http.Error(w, "missing data", http.StatusBadRequest)
				return
			}
			filename := payload.Fn
			if strings.TrimSpace(filename) == "" {
				filename = payload.FilePath
			}
			if strings.TrimSpace(filename) == "" {
				filename = "data.pkl"
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-pickle-%d", nextID)
			data[cid] = append([]byte(nil), payload.Data...)
			fileNames[cid] = filename
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/calculate_json_cid":
			defer r.Body.Close()
			var payload struct {
				Data     json.RawMessage `json:"data"`
				Fn       string          `json:"fn"`
				FilePath string          `json:"file_path"`
				Secret   string          `json:"secret"`
				Nonce    int             `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Nonce == 0 {
				http.Error(w, "nonce required", http.StatusBadRequest)
				return
			}
			cid := fmt.Sprintf("CID-json-calc-%d", payload.Nonce)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/calculate_pickle_cid":
			defer r.Body.Close()
			var payload struct {
				Data     json.RawMessage `json:"data"`
				Fn       string          `json:"fn"`
				FilePath string          `json:"file_path"`
				Secret   string          `json:"secret"`
				Nonce    int             `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Nonce == 0 {
				http.Error(w, "nonce required", http.StatusBadRequest)
				return
			}
			cid := fmt.Sprintf("CID-pickle-calc-%d", payload.Nonce)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
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
	})
	srv := newLocalHTTPServer(t, handler)
	defer srv.Close()

	client, err := r1fs.New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	payload := strings.NewReader("hello world")
	cid, err := client.AddFileBase64(ctx, payload, &r1fs.DataOptions{FilePath: "/tmp/hello.txt"})
	if err != nil {
		t.Fatalf("AddFileBase64: %v", err)
	}
	if cid == "" {
		t.Fatalf("AddFileBase64 returned empty cid")
	}

	content, filename, err := client.GetFileBase64(ctx, cid, "")
	if err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("GetFileBase64 mismatch: %q", string(content))
	}
	if filename != "download.bin" {
		t.Fatalf("GetFileBase64 filename mismatch: %q", filename)
	}

	streamPayload := strings.NewReader("stream upload")
	streamCID, err := client.AddFile(ctx, streamPayload, &r1fs.DataOptions{Filename: "stream.txt", Secret: "s3"})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if streamCID == "" {
		t.Fatalf("AddFile returned empty cid")
	}
	loc, err := client.GetFile(ctx, streamCID, "s3")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if loc == nil || loc.Filename != "stream.txt" || loc.Path == "" {
		t.Fatalf("unexpected file location: %#v", loc)
	}
	overridePayload := strings.NewReader("override")
	overrideCID, err := client.AddFile(ctx, overridePayload, &r1fs.DataOptions{Filename: "remote.bin"})
	if err != nil {
		t.Fatalf("AddFile with override: %v", err)
	}
	overrideLoc, err := client.GetFile(ctx, overrideCID, "")
	if err != nil {
		t.Fatalf("GetFile override: %v", err)
	}
	if overrideLoc.Filename != "remote.bin" {
		t.Fatalf("expected overridden filename, got %#v", overrideLoc)
	}
	jsonOptsNonce := 7
	jsonCID, err := client.AddJSON(ctx, map[string]any{"name": "ratio1"}, &r1fs.DataOptions{Filename: "doc.json", Secret: "sec", Nonce: &jsonOptsNonce})
	if err != nil {
		t.Fatalf("AddJSON: %v", err)
	}
	if strings.TrimSpace(jsonCID) == "" {
		t.Fatalf("AddJSON returned empty cid")
	}
	pickleNonce := 13
	pickleCID, err := client.AddPickle(ctx, map[string]int{"v": 1}, &r1fs.DataOptions{Nonce: &pickleNonce})
	if err != nil {
		t.Fatalf("AddPickle: %v", err)
	}
	if strings.TrimSpace(pickleCID) == "" {
		t.Fatalf("AddPickle returned empty cid")
	}
	calcJSONCID, err := client.CalculateJSONCID(ctx, map[string]string{"kind": "json"}, 42, &r1fs.DataOptions{Secret: "sec"})
	if err != nil {
		t.Fatalf("CalculateJSONCID: %v", err)
	}
	if calcJSONCID != "CID-json-calc-42" {
		t.Fatalf("unexpected JSON cid: %s", calcJSONCID)
	}
	calcPickleCID, err := client.CalculatePickleCID(ctx, map[string]string{"kind": "pickle"}, 56, nil)
	if err != nil {
		t.Fatalf("CalculatePickleCID: %v", err)
	}
	if calcPickleCID != "CID-pickle-calc-56" {
		t.Fatalf("unexpected pickle cid: %s", calcPickleCID)
	}

	yamlPayload := map[string]any{"name": "ratio1", "count": 2}
	yamlCID, err := client.AddYAML(ctx, yamlPayload, &r1fs.DataOptions{Filename: "config.yaml"})
	if err != nil {
		t.Fatalf("AddYAML: %v", err)
	}
	if strings.TrimSpace(yamlCID) == "" {
		t.Fatalf("AddYAML returned empty cid")
	}
	var yamlDoc map[string]any
	doc, err := client.GetYAML(ctx, yamlCID, "", &yamlDoc)
	if err != nil {
		t.Fatalf("GetYAML: %v", err)
	}
	if doc == nil || yamlDoc["name"] != "ratio1" {
		t.Fatalf("unexpected YAML document: %#v value=%#v", doc, yamlDoc)
	}
	if _, err := client.GetYAML(ctx, "missing-yaml", "", nil); err == nil {
		t.Fatalf("expected error for missing YAML document")
	}
}

type testServer struct {
	URL      string
	listener net.Listener
	server   *http.Server
}

func (s *testServer) Close() {
	_ = s.server.Shutdown(context.Background())
	_ = s.listener.Close()
}

func newLocalHTTPServer(t *testing.T, handler http.Handler) *testServer {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("network disabled for tests: %v", err)
	}
	srv := &http.Server{Handler: handler}
	ts := &testServer{
		URL:      "http://" + ln.Addr().String(),
		listener: ln,
		server:   srv,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Logf("test server serve error: %v", err)
		}
	}()
	return ts
}
