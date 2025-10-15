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
	"strconv"
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
	cid, err := fs.AddFileBase64(ctx, bytes.NewReader(data), &r1fs.DataOptions{FilePath: "/docs/http.txt"})
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

	cid, err := fs.AddYAML(ctx, map[string]string{"env": "auto"}, &r1fs.DataOptions{Filename: "env.yaml"})
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
	cid, err := fs.AddFile(ctx, bytes.NewReader(content), &r1fs.DataOptions{Filename: "mock.bin"})
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
		Data   []byte
		Name   string
		Secret string
		Nonce  *int
	}
	type yamlRecord struct {
		Data   json.RawMessage
		Secret string
		Nonce  *int
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
				FileBase64 string `json:"file_base64_str"`
				Filename   string `json:"filename"`
				FilePath   string `json:"file_path"`
				Secret     string `json:"secret"`
				Nonce      *int   `json:"nonce"`
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
			name := strings.TrimSpace(req.Filename)
			if name == "" {
				name = strings.TrimSpace(req.FilePath)
			}
			if name == "" {
				http.Error(w, "filename is required", http.StatusBadRequest)
				return
			}
			cid := newCID()
			rec := fileRecord{
				Data:   data,
				Name:   name,
				Secret: strings.TrimSpace(req.Secret),
				Nonce:  req.Nonce,
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
				"filename":        rec.Name,
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
			name := strings.TrimSpace(header.Filename)
			secret := ""
			var nonce *int
			if metaRaw := strings.TrimSpace(r.FormValue("body_json")); metaRaw != "" {
				var meta map[string]any
				if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
					http.Error(w, fmt.Sprintf("invalid body_json: %v", err), http.StatusBadRequest)
					return
				}
				if fn, ok := meta["fn"].(string); ok && strings.TrimSpace(fn) != "" {
					name = fn
				}
				if fp, ok := meta["file_path"].(string); ok && strings.TrimSpace(fp) != "" {
					name = fp
				}
				if s, ok := meta["secret"].(string); ok {
					secret = strings.TrimSpace(s)
				}
				if rawNonce, ok := meta["nonce"]; ok {
					if ptr := toIntPointer(rawNonce); ptr != nil {
						nonce = ptr
					}
				}
			}
			if name == "" {
				http.Error(w, "filename is required", http.StatusBadRequest)
				return
			}
			cid := newCID()
			rec := fileRecord{Data: data, Name: name, Secret: secret, Nonce: nonce}
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
			meta := map[string]any{"filename": rec.Name}
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
				FilePath string          `json:"file_path"`
				Nonce    *int            `json:"nonce"`
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
			rec := yamlRecord{
				Data:   append([]byte(nil), bytes.TrimSpace(req.Data)...),
				Secret: strings.TrimSpace(req.Secret),
				Nonce:  req.Nonce,
			}
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

func toIntPointer(value any) *int {
	switch v := value.(type) {
	case float64:
		i := int(v)
		return &i
	case float32:
		i := int(v)
		return &i
	case int:
		i := v
		return &i
	case int64:
		i := int(v)
		return &i
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			i := int(parsed)
			return &i
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		if parsed, err := strconv.Atoi(s); err == nil {
			i := parsed
			return &i
		}
	}
	return nil
}
