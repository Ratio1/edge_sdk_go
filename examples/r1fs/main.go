package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func main() {
	server := newR1FSServer()
	defer server.Close()

	client, err := r1fs.New(server.URL)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	fmt.Println("== AddFileBase64 and GetFileBase64 ==")
	payload := []byte("hello from r1fs")
	base64CID, err := client.AddFileBase64(ctx, bytes.NewReader(payload), &r1fs.DataOptions{FilePath: "assets/hello.txt"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("uploaded CID: %s size: %d\n", base64CID, len(payload))

	data, filename, err := client.GetFileBase64(ctx, base64CID, "")
	if err != nil {
		panic(err)
	}
	fmt.Printf("downloaded contents: %q (filename: %s)\n", string(data), filename)

	fmt.Println("\n== AddFile (multipart) and GetFile metadata ==")
	fileCID, err := client.AddFile(ctx, bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef}), &r1fs.DataOptions{Filename: "report.bin"})
	if err != nil {
		panic(err)
	}
	loc, err := client.GetFile(ctx, fileCID, "")
	if err != nil {
		panic(err)
	}
	fmt.Printf("download path: %s filename: %s meta:%v\n", loc.Path, loc.Filename, loc.Meta)

	fmt.Println("\n== AddYAML and GetYAML ==")
	cid, err := client.AddYAML(ctx, map[string]any{"service": "r1fs", "enabled": true}, &r1fs.DataOptions{Filename: "config.yaml"})
	if err != nil {
		panic(err)
	}
	var yamlData map[string]any
	if _, err := client.GetYAML(ctx, cid, "", &yamlData); err != nil {
		panic(err)
	}
	fmt.Printf("yaml payload: %v\n", yamlData)

	fmt.Println("\n== JSON/Pickle helpers ==")
	jsonNonce := 21
	jsonCID2, err := client.AddJSON(ctx, map[string]any{"type": "json", "enabled": true}, &r1fs.DataOptions{Filename: "data.json", Secret: "demo", Nonce: &jsonNonce})
	if err != nil {
		panic(err)
	}
	calcJSON, err := client.CalculateJSONCID(ctx, map[string]any{"type": "json", "enabled": true}, 42, &r1fs.DataOptions{Secret: "demo"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("json cid: %s calculated:%s\n", jsonCID2, calcJSON)

	pickleCID, err := client.AddPickle(ctx, map[string]int{"version": 1}, nil)
	if err != nil {
		panic(err)
	}
	calcPickle, err := client.CalculatePickleCID(ctx, map[string]int{"version": 1}, 99, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("pickle cid: %s calculated:%s\n", pickleCID, calcPickle)
}

func newR1FSServer() *httptest.Server {
	var (
		mu        sync.Mutex
		files     = map[string][]byte{}
		filenames = map[string]string{}
		nextID    = 0
		yamlDocs  = map[string]map[string]any{}
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/add_file_base64":
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
			data, err := base64.StdEncoding.DecodeString(payload.FileBase64)
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
			cid := fmt.Sprintf("CID-%d", nextID)
			files[cid] = data
			filenames[cid] = name
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"cid": cid,
			})

		case "/get_file_base64":
			var req struct {
				CID string `json:"cid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			data, ok := files[req.CID]
			filename := filenames[req.CID]
			mu.Unlock()
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"file_base64_str": base64.StdEncoding.EncodeToString(data),
				"filename":        filename,
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
			data, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			filename := header.Filename
			if meta := strings.TrimSpace(r.FormValue("body_json")); meta != "" {
				var payload map[string]any
				if err := json.Unmarshal([]byte(meta), &payload); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if fn, ok := payload["fn"].(string); ok && strings.TrimSpace(fn) != "" {
					filename = fn
				}
				if fp, ok := payload["file_path"].(string); ok && strings.TrimSpace(fp) != "" {
					filename = fp
				}
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-file-%d", nextID)
			files[cid] = data
			filenames[cid] = filename
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/get_file":
			cid := r.URL.Query().Get("cid")
			mu.Lock()
			_, ok := files[cid]
			filename := filenames[cid]
			mu.Unlock()
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			path := filepath.Join("/tmp", filename)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"file_path": path,
					"meta": map[string]any{
						"file":     path,
						"filename": filename,
					},
				},
			})

		case "/add_yaml":
			var payload struct {
				Data json.RawMessage `json:"data"`
				Fn   string          `json:"fn"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var decoded map[string]any
			if err := json.Unmarshal(payload.Data, &decoded); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-yaml-%d", nextID)
			yamlDocs[cid] = decoded
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/add_json":
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Fn     string          `json:"fn"`
				Secret string          `json:"secret"`
				Nonce  *int            `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload.Data) == 0 {
				http.Error(w, "missing data", http.StatusBadRequest)
				return
			}
			name := payload.Fn
			if strings.TrimSpace(name) == "" {
				name = "data.json"
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-json-%d", nextID)
			files[cid] = append([]byte(nil), payload.Data...)
			filenames[cid] = name
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/add_pickle":
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Fn     string          `json:"fn"`
				Secret string          `json:"secret"`
				Nonce  *int            `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload.Data) == 0 {
				http.Error(w, "missing data", http.StatusBadRequest)
				return
			}
			name := payload.Fn
			if strings.TrimSpace(name) == "" {
				name = "data.pkl"
			}
			mu.Lock()
			nextID++
			cid := fmt.Sprintf("CID-pickle-%d", nextID)
			files[cid] = append([]byte(nil), payload.Data...)
			filenames[cid] = name
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/calculate_json_cid":
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Nonce  int             `json:"nonce"`
				Fn     string          `json:"fn"`
				Secret string          `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Nonce == 0 {
				http.Error(w, "nonce is required", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": fmt.Sprintf("CID-json-calc-%d", payload.Nonce)},
			})

		case "/calculate_pickle_cid":
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Nonce  int             `json:"nonce"`
				Fn     string          `json:"fn"`
				Secret string          `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Nonce == 0 {
				http.Error(w, "nonce is required", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": fmt.Sprintf("CID-pickle-calc-%d", payload.Nonce)},
			})

		case "/get_yaml":
			cid := r.URL.Query().Get("cid")
			mu.Lock()
			data, ok := yamlDocs[cid]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				_ = json.NewEncoder(w).Encode("error")
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"file_data": data})

		default:
			http.NotFound(w, r)
		}
	}))
}
