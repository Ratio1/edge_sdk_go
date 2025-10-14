package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
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
	if err := cs.Set(ctx, "jobs:1", map[string]any{"status": "queued"}, nil); err != nil {
		return err
	}

	var itemValue map[string]any
	if _, err := cs.Get(ctx, "jobs:1", &itemValue); err != nil {
		return err
	}
	fmt.Println("cstore get:", itemValue)

	data := []byte("hello http")
	cid, err := fs.AddFileBase64(ctx, "/docs/http.txt", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		return err
	}
	fmt.Printf("r1fs add_file_base64: cid=%s size=%d\n", cid, len(data))

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
	if err := cs.HSet(ctx, "jobs", "1", map[string]int{"attempts": 1}, nil); err != nil {
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
	if err := cs.Set(ctx, "users:1", map[string]string{"name": "mock"}, nil); err != nil {
		return err
	}
	status, err := cs.GetStatus(ctx)
	if err != nil {
		return err
	}
	if status != nil {
		for _, key := range status.Keys {
			var value map[string]string
			if _, err := cs.Get(ctx, key, &value); err != nil {
				return fmt.Errorf("decode item %s: %w", key, err)
			}
			fmt.Printf("mock cstore item %s -> %v\n", key, value)
		}
	}

	content := []byte("mock payload")
	cid, err := fs.AddFile(ctx, "mock.bin", bytes.NewReader(content), int64(len(content)), nil)
	if err != nil {
		return err
	}
	loc, err := fs.GetFile(ctx, cid, "")
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
		Data        []byte
		Filename    string
		Secret      string
		Metadata    map[string]string
		ContentType string
	}
	type yamlRecord struct {
		Data   json.RawMessage
		Secret string
	}

	var (
		mu        sync.Mutex
		kvStore   = map[string]json.RawMessage{}
		hashStore = map[string]map[string]json.RawMessage{}
		files     = map[string]fileRecord{}
		yamlDocs  = map[string]yamlRecord{}
		nextCID   int
	)

	newCID := func() string {
		nextCID++
		return fmt.Sprintf("/cid/%08d", nextCID)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			var req struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.Key) == "" {
				http.Error(w, "key is required", http.StatusBadRequest)
				return
			}
			value := json.RawMessage(bytes.TrimSpace(req.Value))
			if len(value) == 0 {
				value = json.RawMessage("null")
			}
			mu.Lock()
			kvStore[req.Key] = append([]byte(nil), value...)
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
			resp := map[string]any{"result": json.RawMessage(value)}
			_ = json.NewEncoder(w).Encode(resp)

		case "/hset":
			var req struct {
				HashKey string          `json:"hkey"`
				Field   string          `json:"key"`
				Value   json.RawMessage `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.HashKey) == "" || strings.TrimSpace(req.Field) == "" {
				http.Error(w, "hkey and key are required", http.StatusBadRequest)
				return
			}
			value := json.RawMessage(bytes.TrimSpace(req.Value))
			if len(value) == 0 {
				value = json.RawMessage("null")
			}
			mu.Lock()
			bucket := hashStore[req.HashKey]
			if bucket == nil {
				bucket = make(map[string]json.RawMessage)
				hashStore[req.HashKey] = bucket
			}
			bucket[req.Field] = append([]byte(nil), value...)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("true"))

		case "/hget":
			hkey := r.URL.Query().Get("hkey")
			field := r.URL.Query().Get("key")
			mu.Lock()
			bucket := hashStore[hkey]
			var value json.RawMessage
			if bucket != nil {
				value = bucket[field]
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if len(value) == 0 {
				_, _ = w.Write([]byte("null"))
				return
			}
			resp := map[string]any{"result": json.RawMessage(value)}
			_ = json.NewEncoder(w).Encode(resp)

		case "/hgetall":
			hkey := r.URL.Query().Get("hkey")
			mu.Lock()
			bucket := hashStore[hkey]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if len(bucket) == 0 {
				_, _ = w.Write([]byte("null"))
				return
			}
			payload := make(map[string]json.RawMessage, len(bucket))
			for k, v := range bucket {
				payload[k] = json.RawMessage(v)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp := map[string]any{"result": json.RawMessage(encoded)}
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
				FileBase64  string            `json:"file_base64_str"`
				Filename    string            `json:"filename"`
				Secret      string            `json:"secret"`
				Metadata    map[string]string `json:"metadata"`
				ContentType string            `json:"content_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.FileBase64) == "" {
				http.Error(w, "file_base64_str is required", http.StatusBadRequest)
				return
			}
			data, err := base64.StdEncoding.DecodeString(req.FileBase64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			filename := strings.TrimSpace(req.Filename)
			if filename == "" {
				filename = fmt.Sprintf("file-%d.bin", len(files)+1)
			}
			cid := newCID()
			rec := fileRecord{
				Data:        data,
				Filename:    filename,
				Secret:      strings.TrimSpace(req.Secret),
				Metadata:    nil,
				ContentType: strings.TrimSpace(req.ContentType),
			}
			if len(req.Metadata) > 0 {
				rec.Metadata = make(map[string]string, len(req.Metadata))
				for k, v := range req.Metadata {
					rec.Metadata[k] = v
				}
			}
			mu.Lock()
			files[cid] = rec
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/get_file_base64":
			var req struct {
				CID    string `json:"cid"`
				Secret string `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			rec, ok := files[req.CID]
			mu.Unlock()
			if !ok || (rec.Secret != "" && rec.Secret != strings.TrimSpace(req.Secret)) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"file_base64_str": base64.StdEncoding.EncodeToString(rec.Data),
				"filename":        rec.Filename,
			})

		case "/add_file":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			metaRaw := strings.TrimSpace(r.FormValue("body_json"))
			rec := fileRecord{Data: data, Filename: header.Filename}
			if metaRaw != "" {
				var meta map[string]any
				if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
					http.Error(w, fmt.Sprintf("invalid body_json: %v", err), http.StatusBadRequest)
					return
				}
				if secret, ok := meta["secret"].(string); ok {
					rec.Secret = secret
				}
				if metadata, ok := meta["metadata"].(map[string]any); ok {
					rec.Metadata = map[string]string{}
					for k, v := range metadata {
						if str, ok := v.(string); ok {
							rec.Metadata[k] = str
						}
					}
				}
				if ct, ok := meta["content_type"].(string); ok {
					rec.ContentType = ct
				}
			}
			if strings.TrimSpace(rec.ContentType) == "" {
				rec.ContentType = http.DetectContentType(data)
			}
			cid := newCID()
			mu.Lock()
			files[cid] = rec
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/get_file":
			cid := r.URL.Query().Get("cid")
			secret := strings.TrimSpace(r.URL.Query().Get("secret"))
			mu.Lock()
			rec, ok := files[cid]
			mu.Unlock()
			if !ok || (rec.Secret != "" && rec.Secret != secret) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			meta := map[string]any{}
			if rec.Filename != "" {
				meta["filename"] = rec.Filename
			}
			if len(rec.Metadata) > 0 {
				inner := make(map[string]string, len(rec.Metadata))
				for k, v := range rec.Metadata {
					inner[k] = v
				}
				meta["metadata"] = inner
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"file_path": cid,
					"meta":      meta,
				},
			})

		case "/add_yaml":
			var req struct {
				Data     json.RawMessage `json:"data"`
				Filename string          `json:"fn"`
				Secret   string          `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(bytes.TrimSpace(req.Data)) == 0 {
				http.Error(w, "data is required", http.StatusBadRequest)
				return
			}
			cid := newCID()
			rec := yamlRecord{Data: append([]byte(nil), bytes.TrimSpace(req.Data)...), Secret: strings.TrimSpace(req.Secret)}
			mu.Lock()
			yamlDocs[cid] = rec
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"cid": cid},
			})

		case "/get_yaml":
			cid := r.URL.Query().Get("cid")
			secret := strings.TrimSpace(r.URL.Query().Get("secret"))
			mu.Lock()
			rec, ok := yamlDocs[cid]
			mu.Unlock()
			if !ok || (rec.Secret != "" && rec.Secret != secret) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if len(rec.Data) == 0 {
				_, _ = w.Write([]byte("null"))
				return
			}
			var payload any
			if err := json.Unmarshal(rec.Data, &payload); err != nil {
				http.Error(w, fmt.Sprintf("decode yaml data: %v", err), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": payload})

		default:
			http.NotFound(w, r)
		}
	}))
}

func mapsKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
