package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	cstoreURLEnv = "EE_CHAINSTORE_API_URL"
	r1fsURLEnv   = "EE_R1FS_API_URL"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(p []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}
	lrw.buf.Write(p)
	return lrw.ResponseWriter.Write(p)
}

func (lrw *loggingResponseWriter) Flush() {
	if f, ok := lrw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func main() {
	cstoreAddr := flag.String("cstore-addr", ":8787", "listen address for cstore API")
	r1fsAddr := flag.String("r1fs-addr", ":8788", "listen address for r1fs API")
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

	wrap := func(next http.HandlerFunc) http.HandlerFunc {
		return loggingMiddleware(withMiddleware(*latency, failCfg, next))
	}

	cstoreMux := http.NewServeMux()
	cstoreMux.HandleFunc("/get_status", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreStatus(w, r, csMock)
	}))
	cstoreMux.HandleFunc("/set", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreSet(w, r, csMock)
	}))
	cstoreMux.HandleFunc("/get", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreGet(w, r, csMock)
	}))
	cstoreMux.HandleFunc("/hset", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreHSet(w, r, csMock)
	}))
	cstoreMux.HandleFunc("/hget", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreHGet(w, r, csMock)
	}))
	cstoreMux.HandleFunc("/hgetall", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleCStoreHGetAll(w, r, csMock)
	}))

	r1fsMux := http.NewServeMux()
	r1fsMux.HandleFunc("/add_file_base64", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSAddFileBase64(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/add_file", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSAddFile(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/get_file_base64", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSGetFileBase64(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/get_file", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSGetFile(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/add_yaml", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSAddYAML(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/add_json", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSAddJSON(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/add_pickle", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSAddPickle(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/calculate_json_cid", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSCalculateJSONCID(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/calculate_pickle_cid", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSCalculatePickleCID(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/get_yaml", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSGetYAML(w, r, fsMock)
	}))
	r1fsMux.HandleFunc("/get_status_r1fs", wrap(func(w http.ResponseWriter, r *http.Request) {
		handleR1FSStatus(w, r, fsMock)
	}))

	cstoreServer := &http.Server{Addr: *cstoreAddr, Handler: cstoreMux}
	r1fsServer := &http.Server{Addr: *r1fsAddr, Handler: r1fsMux}

	go func() {
		if err := cstoreServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("cstore server failed: %v", err)
		}
	}()

	log.Printf("ratio1-sandbox cstore listening on %s", *cstoreAddr)
	log.Printf("ratio1-sandbox r1fs listening on %s", *r1fsAddr)
	fmt.Println()
	fmt.Println("export R1_RUNTIME_MODE=http")
	fmt.Printf("export %s=http://%s\n", cstoreURLEnv, hostFromAddr(*cstoreAddr))
	fmt.Printf("export %s=http://%s\n", r1fsURLEnv, hostFromAddr(*r1fsAddr))
	fmt.Println()

	if err := r1fsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("r1fs server failed: %v", err)
	}
}

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var reqBody []byte
		if r.Body != nil {
			var err error
			reqBody, err = io.ReadAll(r.Body)
			if err != nil {
				log.Printf("sandbox: read request body error: %v", err)
			}
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		lrw := &loggingResponseWriter{ResponseWriter: w}
		next(lrw, r)

		duration := time.Since(start)
		qs := r.URL.RawQuery
		if qs != "" {
			qs = "?" + qs
		}
		status := lrw.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("%s %s%s -> %d (%s)\n  Request: %s\n  Response: %s\n",
			r.Method,
			r.URL.Path,
			qs,
			status,
			duration.Truncate(time.Microsecond),
			formatBodyForLog(reqBody),
			formatBodyForLog(lrw.buf.Bytes()),
		)
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
			writeError(w, status, "failure injected", nil)
			return
		}
		next(w, r)
	}
}

func handleCStoreStatus(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	ctx := r.Context()
	status, err := cstoremock.GetStatus(ctx, store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cstore get_status failed", err)
		return
	}
	var keys []string
	if status != nil {
		keys = append([]string(nil), status.Keys...)
	}
	writeResult(w, map[string]any{"keys": keys})
}

func handleCStoreSet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if payload.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required", nil)
		return
	}
	if err := cstoremock.Set(r.Context(), store, payload.Key, payload.Value, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "cstore set failed", err)
		return
	}
	writeResult(w, true)
}

func handleCStoreGet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing key parameter", nil)
		return
	}
	item, err := cstoremock.Get[any](r.Context(), store, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cstore get failed", err)
		return
	}
	if item == nil {
		writeResult(w, nil)
		return
	}
	writeResult(w, item.Value)
}

func handleCStoreHSet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		HashKey string `json:"hkey"`
		Field   string `json:"key"`
		Value   any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if strings.TrimSpace(payload.HashKey) == "" || strings.TrimSpace(payload.Field) == "" {
		writeError(w, http.StatusBadRequest, "hkey and key are required", nil)
		return
	}

	if err := cstoremock.HSet(r.Context(), store, payload.HashKey, payload.Field, payload.Value, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "cstore hset failed", err)
		return
	}
	writeResult(w, true)
}

func handleCStoreHGet(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	hashKey := r.URL.Query().Get("hkey")
	field := r.URL.Query().Get("key")
	if strings.TrimSpace(hashKey) == "" || strings.TrimSpace(field) == "" {
		writeError(w, http.StatusBadRequest, "hkey and key are required", nil)
		return
	}
	item, err := cstoremock.HGet[any](r.Context(), store, hashKey, field)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cstore hget failed", err)
		return
	}
	if item == nil {
		writeResult(w, nil)
		return
	}
	writeResult(w, item.Value)
}

func handleCStoreHGetAll(w http.ResponseWriter, r *http.Request, store *cstoremock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	hashKey := r.URL.Query().Get("hkey")
	if strings.TrimSpace(hashKey) == "" {
		writeError(w, http.StatusBadRequest, "hkey is required", nil)
		return
	}
	items, err := cstoremock.HGetAll[string](r.Context(), store, hashKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cstore hgetall failed", err)
		return
	}
	if len(items) == 0 {
		writeResult(w, nil)
		return
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		result[item.Field] = item.Value
	}
	writeResult(w, result)
}

func handleR1FSAddFileBase64(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Base64   string `json:"file_base64_str"`
		Filename string `json:"filename"`
		FilePath string `json:"file_path"`
		Secret   string `json:"secret"`
		Nonce    *int   `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if payload.Base64 == "" {
		writeError(w, http.StatusBadRequest, "file_base64_str is required", nil)
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload.Base64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid base64 payload", err)
		return
	}
	if strings.TrimSpace(payload.Filename) == "" && strings.TrimSpace(payload.FilePath) == "" {
		writeError(w, http.StatusBadRequest, "filename or file_path is required", nil)
		return
	}
	opts := &r1fs.DataOptions{Filename: strings.TrimSpace(payload.Filename), FilePath: strings.TrimSpace(payload.FilePath)}
	if strings.TrimSpace(payload.Secret) != "" {
		opts.Secret = payload.Secret
	}
	if payload.Nonce != nil {
		opts.Nonce = payload.Nonce
	}
	cid, err := fs.AddFileBase64(r.Context(), bytes.NewReader(data), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs add_file_base64 failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSGetFileBase64(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if payload.CID == "" {
		writeError(w, http.StatusBadRequest, "cid is required", nil)
		return
	}
	data, filename, err := fs.GetFileBase64(r.Context(), payload.CID, "")
	if err != nil {
		writeError(w, http.StatusNotFound, "r1fs get_file_base64 failed", err)
		return
	}
	if filename == "" {
		filename = strings.TrimPrefix(payload.CID, "/")
	}
	writeResult(w, map[string]any{
		"file_base64_str": base64.StdEncoding.EncodeToString(data),
		"filename":        filename,
	})
}

func handleR1FSStatus(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	writeResult(w, fs.Status())
}

func handleR1FSAddFile(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "unable to parse multipart form", err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file part is required", err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read uploaded file", err)
		return
	}
	metaRaw := r.FormValue("body_json")
	opts := &r1fs.DataOptions{}
	if name := strings.TrimSpace(header.Filename); name != "" {
		opts.Filename = name
	}
	if strings.TrimSpace(metaRaw) != "" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body_json", err)
			return
		}
		if secret, ok := meta["secret"].(string); ok {
			opts.Secret = secret
		}
		if nonce, ok := meta["nonce"]; ok {
			if value, ok := coerceToInt(nonce); ok {
				tmp := value
				opts.Nonce = &tmp
			}
		}
		if fp, ok := meta["file_path"].(string); ok && strings.TrimSpace(fp) != "" {
			opts.FilePath = fp
		}
		if fn, ok := meta["fn"].(string); ok && strings.TrimSpace(fn) != "" {
			opts.Filename = fn
		}
	}
	if strings.TrimSpace(opts.Filename) == "" && strings.TrimSpace(opts.FilePath) == "" {
		writeError(w, http.StatusBadRequest, "filename or file_path is required", nil)
		return
	}
	cid, err := fs.AddFile(r.Context(), bytes.NewReader(data), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs add_file failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSGetFile(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	cid := r.URL.Query().Get("cid")
	secret := r.URL.Query().Get("secret")
	if strings.TrimSpace(cid) == "" {
		writeError(w, http.StatusBadRequest, "cid is required", nil)
		return
	}
	loc, err := fs.GetFile(r.Context(), cid, secret)
	if err != nil {
		writeError(w, http.StatusNotFound, "r1fs get_file failed", err)
		return
	}
	writeResult(w, map[string]any{
		"file_path": loc.Path,
		"meta":      loc.Meta,
	})
}

func handleR1FSAddYAML(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Data     json.RawMessage `json:"data"`
		Filename string          `json:"fn"`
		FilePath string          `json:"file_path"`
		Secret   string          `json:"secret"`
		Nonce    *int            `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if len(bytes.TrimSpace(payload.Data)) == 0 {
		writeError(w, http.StatusBadRequest, "data is required", nil)
		return
	}
	var value any
	if err := json.Unmarshal(payload.Data, &value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid data payload", err)
		return
	}
	ops := &r1fs.DataOptions{Filename: payload.Filename, FilePath: payload.FilePath, Secret: payload.Secret, Nonce: payload.Nonce}
	cid, err := fs.AddYAML(r.Context(), value, ops)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs add_yaml failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSAddJSON(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Data     json.RawMessage `json:"data"`
		Fn       string          `json:"fn"`
		FilePath string          `json:"file_path"`
		Nonce    *int            `json:"nonce"`
		Secret   string          `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if len(bytes.TrimSpace(payload.Data)) == 0 {
		writeError(w, http.StatusBadRequest, "data is required", nil)
		return
	}
	var value any
	if err := json.Unmarshal(payload.Data, &value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid data payload", err)
		return
	}
	opts := &r1fs.DataOptions{Filename: payload.Fn, FilePath: payload.FilePath, Secret: payload.Secret, Nonce: payload.Nonce}
	cid, err := fs.AddJSON(r.Context(), value, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs add_json failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSAddPickle(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Data     json.RawMessage `json:"data"`
		Fn       string          `json:"fn"`
		FilePath string          `json:"file_path"`
		Nonce    *int            `json:"nonce"`
		Secret   string          `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if len(bytes.TrimSpace(payload.Data)) == 0 {
		writeError(w, http.StatusBadRequest, "data is required", nil)
		return
	}
	var value any
	if err := json.Unmarshal(payload.Data, &value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid data payload", err)
		return
	}
	opts := &r1fs.DataOptions{Filename: payload.Fn, FilePath: payload.FilePath, Secret: payload.Secret, Nonce: payload.Nonce}
	cid, err := fs.AddPickle(r.Context(), value, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs add_pickle failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSCalculateJSONCID(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Data     json.RawMessage `json:"data"`
		Fn       string          `json:"fn"`
		FilePath string          `json:"file_path"`
		Secret   string          `json:"secret"`
		Nonce    int             `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if payload.Nonce == 0 {
		writeError(w, http.StatusBadRequest, "nonce is required", nil)
		return
	}
	var value any
	if len(payload.Data) > 0 {
		if err := json.Unmarshal(payload.Data, &value); err != nil {
			writeError(w, http.StatusBadRequest, "invalid data payload", err)
			return
		}
	}
	opts := &r1fs.DataOptions{Filename: payload.Fn, FilePath: payload.FilePath, Secret: payload.Secret}
	cid, err := fs.CalculateJSONCID(r.Context(), value, payload.Nonce, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs calculate_json_cid failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSCalculatePickleCID(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var payload struct {
		Data     json.RawMessage `json:"data"`
		Fn       string          `json:"fn"`
		FilePath string          `json:"file_path"`
		Secret   string          `json:"secret"`
		Nonce    int             `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload", err)
		return
	}
	if payload.Nonce == 0 {
		writeError(w, http.StatusBadRequest, "nonce is required", nil)
		return
	}
	var value any
	if len(payload.Data) > 0 {
		if err := json.Unmarshal(payload.Data, &value); err != nil {
			writeError(w, http.StatusBadRequest, "invalid data payload", err)
			return
		}
	}
	opts := &r1fs.DataOptions{Filename: payload.Fn, FilePath: payload.FilePath, Secret: payload.Secret}
	cid, err := fs.CalculatePickleCID(r.Context(), value, payload.Nonce, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs calculate_pickle_cid failed", err)
		return
	}
	writeResult(w, map[string]any{"cid": cid})
}

func handleR1FSGetYAML(w http.ResponseWriter, r *http.Request, fs *r1fsmock.Mock) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	cid := r.URL.Query().Get("cid")
	secret := r.URL.Query().Get("secret")
	if strings.TrimSpace(cid) == "" {
		writeError(w, http.StatusBadRequest, "cid is required", nil)
		return
	}
	data, err := fs.GetYAML(r.Context(), cid, secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "r1fs get_yaml failed", err)
		return
	}
	if len(data) == 0 {
		writeResult(w, nil)
		return
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "decode yaml payload failed", err)
		return
	}
	writeResult(w, payload)
}

func coerceToInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed), true
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func hostFromAddr(addr string) string {
	host := strings.TrimSpace(addr)
	if host == "" {
		return "localhost"
	}
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	return host
}

func writeResult(w http.ResponseWriter, payload any) {
	writeJSON(w, map[string]any{"result": payload})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("sandbox: encode response error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	body := struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
			Detail  string `json:"detail,omitempty"`
		} `json:"error"`
	}{}
	body.Error.Status = status
	body.Error.Message = message
	body.Error.Detail = detail
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("sandbox: encode error response error: %v", err)
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

func formatBodyForLog(body []byte) string {
	if len(body) == 0 {
		return "<empty>"
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "<whitespace>"
	}
	return trimmed
}
