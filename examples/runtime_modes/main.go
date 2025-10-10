package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func main() {
	fmt.Println("== HTTP mode via env helpers ==")
	if err := demoHTTP(); err != nil {
		log.Fatalf("http demo: %v", err)
	}

	fmt.Println("\n== Auto mode (falls back to HTTP when URLs are present) ==")
	if err := demoAuto(); err != nil {
		log.Fatalf("auto demo: %v", err)
	}

	fmt.Println("\n== Mock mode using in-memory stores ==")
	if err := demoMock(); err != nil {
		log.Fatalf("mock demo: %v", err)
	}
}

func demoHTTP() error {
	server := newSandboxServer()
	defer server.Close()

	setEnv(map[string]string{
		"R1_RUNTIME_MODE":       "http",
		"EE_CHAINSTORE_API_URL": server.URL,
		"EE_R1FS_API_URL":       server.URL,
	})
	defer unsetEnv("R1_RUNTIME_MODE", "EE_CHAINSTORE_API_URL", "EE_R1FS_API_URL")

	cs, fs, mode, err := bootstrapFromEnv()
	if err != nil {
		return err
	}
	fmt.Println("resolved mode:", mode)

	ctx := context.Background()
	if _, err := cs.Put(ctx, "jobs:1", map[string]any{"status": "queued"}, nil); err != nil {
		return err
	}
	var itemValue map[string]any
	if _, err := cs.Get(ctx, "jobs:1", &itemValue); err != nil {
		return err
	}
	fmt.Println("cstore get:", itemValue)

	data := []byte("hello http")
	stat, err := fs.Upload(ctx, "/docs/http.txt", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		return err
	}
	fmt.Printf("r1fs upload: cid=%s size=%d\n", stat.Path, stat.Size)

	return nil
}

func demoAuto() error {
	server := newSandboxServer()
	defer server.Close()

	setEnv(map[string]string{
		"R1_RUNTIME_MODE":       "auto",
		"EE_CHAINSTORE_API_URL": server.URL,
		"EE_R1FS_API_URL":       server.URL,
	})
	defer unsetEnv("R1_RUNTIME_MODE", "EE_CHAINSTORE_API_URL", "EE_R1FS_API_URL")

	cs, fs, mode, err := bootstrapFromEnv()
	if err != nil {
		return err
	}
	fmt.Println("resolved mode:", mode)

	ctx := context.Background()
	if _, err := cs.HSet(ctx, "jobs", "1", map[string]int{"attempts": 1}, nil); err != nil {
		return err
	}
	var hItemValue map[string]int
	if _, err := cs.HGet(ctx, "jobs", "1", &hItemValue); err != nil {
		return err
	}
	fmt.Println("cstore hget:", hItemValue)

	cid, err := fs.AddYAML(ctx, map[string]string{"env": "auto"}, &r1fs.YAMLOptions{Filename: "env.yaml"})
	if err != nil {
		return err
	}
	var yamlData map[string]string
	if _, err := fs.GetYAML(ctx, cid, "", &yamlData); err != nil {
		return err
	}
	fmt.Println("r1fs get_yaml:", yamlData)

	return nil
}

func demoMock() error {
	setEnv(map[string]string{
		"R1_RUNTIME_MODE": "mock",
	})
	defer unsetEnv("R1_RUNTIME_MODE")

	cs, fs, mode, err := bootstrapFromEnv()
	if err != nil {
		return err
	}
	fmt.Println("resolved mode:", mode)

	ctx := context.Background()
	if _, err := cs.Put(ctx, "users:1", map[string]string{"name": "mock"}, nil); err != nil {
		return err
	}
	list, err := cs.List(ctx, "", "", 0)
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		var value map[string]string
		if err := json.Unmarshal(item.Value, &value); err != nil {
			return fmt.Errorf("decode list item %s: %w", item.Key, err)
		}
		fmt.Printf("mock cstore item %s -> %v\n", item.Key, value)
	}

	content := []byte("mock payload")
	stat, err := fs.AddFile(ctx, "mock.bin", bytes.NewReader(content), int64(len(content)), nil)
	if err != nil {
		return err
	}
	loc, err := fs.GetFile(ctx, stat.Path, "")
	if err != nil {
		return err
	}
	fmt.Println("mock r1fs file path:", loc.Path)

	return nil
}

func setEnv(values map[string]string) {
	for k, v := range values {
		if err := os.Setenv(k, v); err != nil {
			panic(err)
		}
	}
}

func unsetEnv(keys ...string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

func bootstrapFromEnv() (*cstore.Client, *r1fs.Client, string, error) {
	cs, cMode, err := cstore.NewFromEnv()
	if err != nil {
		return nil, nil, "", fmt.Errorf("bootstrap cstore: %w", err)
	}
	fs, fMode, err := r1fs.NewFromEnv()
	if err != nil {
		return nil, nil, "", fmt.Errorf("bootstrap r1fs: %w", err)
	}
	mode := cMode
	if mode == "" {
		mode = fMode
	}
	if cMode != "" && fMode != "" && cMode != fMode {
		return nil, nil, "", fmt.Errorf("resolve runtime mode: mismatch (cstore=%s, r1fs=%s)", cMode, fMode)
	}
	return cs, fs, mode, nil
}

func newSandboxServer() *httptest.Server {
	type fileRecord struct {
		Data     []byte
		Filename string
	}

	var (
		mu      sync.Mutex
		kvStore = map[string]string{}
		files   = map[string]fileRecord{}
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			var req struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			kvStore[req.Key] = req.Value
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("true"))

		case "/get":
			key := r.URL.Query().Get("key")
			mu.Lock()
			value, ok := kvStore[key]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !ok {
				_, _ = w.Write([]byte("null"))
				return
			}
			resp := map[string]any{"result": value}
			_ = json.NewEncoder(w).Encode(resp)

		case "/get_status":
			mu.Lock()
			keys := mapsKeys(kvStore)
			mu.Unlock()
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"keys": keys},
			})

		case "/add_file_base64":
			var req struct {
				FileBase64 string `json:"file_base64_str"`
				Filename   string `json:"filename"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			data, err := base64.StdEncoding.DecodeString(req.FileBase64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			cid := "CID-" + req.Filename
			mu.Lock()
			files[cid] = fileRecord{Data: data, Filename: req.Filename}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
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
			rec, ok := files[req.CID]
			mu.Unlock()
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"file_base64_str": base64.StdEncoding.EncodeToString(rec.Data),
				"filename":        rec.Filename,
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func mapsKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
