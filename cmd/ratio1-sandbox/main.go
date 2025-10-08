package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	cstoremock "github.com/Ratio1/ratio1_sdk_go/pkg/cstore/mock"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	r1fsmock "github.com/Ratio1/ratio1_sdk_go/pkg/r1fs/mock"
)

type failConfig struct {
	rate float64
	code int
}

const (
	cstoreURLEnv = "CSTORE_API_URL"
	r1fsURLEnv   = "R1FS_API_URL"
)

func main() {
	addr := flag.String("addr", ":8787", "listen address")
	kvSeed := flag.String("kv-seed", "", "path to JSON seed for cstore mock")
	fsSeed := flag.String("fs-seed", "", "path to JSON seed for r1fs mock")
	latency := flag.Duration("latency", 0, "artificial latency to inject per request")
	fail := flag.String("fail", "", "failure injection (rate=<float>,code=<httpStatus>)")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	csMock := cstoremock.New()
	if *kvSeed != "" {
		entries, err := devseed.LoadCStoreSeed(*kvSeed)
		if err != nil {
			log.Fatalf("load cstore seed: %v", err)
		}
		if err := csMock.Seed(entries); err != nil {
			log.Fatalf("apply cstore seed: %v", err)
		}
	}

	fsMock := r1fsmock.New()
	if *fsSeed != "" {
		entries, err := devseed.LoadR1FSSeed(*fsSeed)
		if err != nil {
			log.Fatalf("load r1fs seed: %v", err)
		}
		if err := fsMock.Seed(entries); err != nil {
			log.Fatalf("apply r1fs seed: %v", err)
		}
	}

	failCfg, err := parseFailConfig(*fail)
	if err != nil {
		log.Fatalf("parse fail flag: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/get_status", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request cstore status")
		handleCStoreStatus(w, r, csMock)
	}))
	mux.HandleFunc("/set", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request cstore set")
		handleCStoreSet(w, r, csMock)
	}))
	mux.HandleFunc("/get", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request cstore get")
		handleCStoreGet(w, r, csMock)
	}))
	mux.HandleFunc("/add_file_base64", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request r1fs add file")
		handleR1FSAddFileBase64(w, r, fsMock)
	}))
	mux.HandleFunc("/get_file_base64", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request r1fs get file")
		handleR1FSGetFileBase64(w, r, fsMock)
	}))
	mux.HandleFunc("/get_status_r1fs", withMiddleware(*latency, failCfg, func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("exec request r1fs get status")
		handleR1FSStatus(w, r, fsMock)
	}))

	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	log.Printf("ratio1-sandbox listening on %s", *addr)
	fmt.Println()
	fmt.Println("export R1_RUNTIME_MODE=http")
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	fmt.Printf("export %s=http://%s\n", cstoreURLEnv, host)
	fmt.Printf("export %s=http://%s\n", r1fsURLEnv, host)
	fmt.Println()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func withMiddleware(delay time.Duration, failCfg failConfig, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		if failCfg.rate > 0 && rand.Float64() < failCfg.rate {
			status := failCfg.code
			if status == 0 {
				status = http.StatusInternalServerError
			}
			http.Error(w, "failure injected", status)
			return
		}
		next(w, r)
	}
}

func handleCStoreStatus(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	ctx := r.Context()
	list, err := cstoremock.List[json.RawMessage](ctx, store, "", "", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	keys := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		keys = append(keys, item.Key)
	}
	writeJSON(w, map[string]any{"keys": keys})
}

func handleCStoreSet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if _, err := cstoremock.Put(r.Context(), store, payload.Key, payload.Value, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, true)
}

func handleCStoreGet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key parameter", http.StatusBadRequest)
		return
	}
	item, err := cstoremock.Get[json.RawMessage](r.Context(), store, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if item == nil {
		writeJSON(w, nil)
		return
	}
	writeJSON(w, item.Value)
}

func handleR1FSAddFileBase64(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Base64   string `json:"file_base64_str"`
		Filename string `json:"filename"`
		Secret   string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Base64 == "" {
		http.Error(w, "file_base64_str is required", http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload.Base64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filename := payload.Filename
	if filename == "" {
		filename = fmt.Sprintf("file-%d.bin", time.Now().UnixNano())
	}
	stat, err := fs.Upload(r.Context(), filename, bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: http.DetectContentType(data)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"cid": stat.Path})
}

func handleR1FSGetFileBase64(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if payload.CID == "" {
		http.Error(w, "cid is required", http.StatusBadRequest)
		return
	}
	var buf bytes.Buffer
	if _, err := fs.Download(r.Context(), payload.CID, &buf); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{
		"file_base64_str": base64.StdEncoding.EncodeToString(buf.Bytes()),
		"filename":        strings.TrimPrefix(payload.CID, "/"),
	})
}

func handleR1FSStatus(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	res, err := fs.List(r.Context(), "/", "", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"files": res.Files})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func parseFailConfig(raw string) (failConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return failConfig{}, nil
	}
	cfg := failConfig{code: http.StatusInternalServerError}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		keyVal := strings.SplitN(part, "=", 2)
		if len(keyVal) != 2 {
			return failConfig{}, fmt.Errorf("invalid fail segment %q", part)
		}
		switch strings.TrimSpace(keyVal[0]) {
		case "rate":
			val, err := strconv.ParseFloat(strings.TrimSpace(keyVal[1]), 64)
			if err != nil {
				return failConfig{}, err
			}
			cfg.rate = val
		case "code":
			val, err := strconv.Atoi(strings.TrimSpace(keyVal[1]))
			if err != nil {
				return failConfig{}, err
			}
			cfg.code = val
		default:
			return failConfig{}, fmt.Errorf("unknown fail key %q", keyVal[0])
		}
	}
	return cfg, nil
}
