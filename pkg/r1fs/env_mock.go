package r1fs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
)

type mockFS struct {
	mu        sync.RWMutex
	files     map[string]*fileEntry
	fileNames map[string]string
	yamlDocs  map[string]json.RawMessage
	now       func() time.Time
}

type fileEntry struct {
	data        []byte
	contentType string
	etag        string
	metadata    map[string]string
	modTime     time.Time
}

func newMockFS() *mockFS {
	return &mockFS{
		files:     make(map[string]*fileEntry),
		fileNames: make(map[string]string),
		yamlDocs:  make(map[string]json.RawMessage),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *mockFS) seed(entries []devseed.R1FSSeedEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range entries {
		if strings.TrimSpace(e.Path) == "" {
			return fmt.Errorf("mock r1fs: seed entry missing path")
		}
		data, err := base64.StdEncoding.DecodeString(e.Base64)
		if err != nil {
			return fmt.Errorf("mock r1fs: decode base64: %w", err)
		}
		path := normalizePath(e.Path)
		entry := &fileEntry{
			data:        append([]byte(nil), data...),
			contentType: e.ContentType,
			etag:        newETag(),
			metadata:    copyStringMap(e.Metadata),
			modTime:     m.now(),
		}
		if e.LastModified != nil {
			entry.modTime = e.LastModified.UTC()
		}
		m.files[path] = entry
		m.fileNames[path] = path
	}
	return nil
}

func (m *mockFS) upload(ctx context.Context, path string, data []byte, size int64, opts *UploadOptions) (*FileStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("mock r1fs: path is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &fileEntry{
		data:        append([]byte(nil), data...),
		contentType: "",
		etag:        newETag(),
		metadata:    optsMetadata(opts),
		modTime:     m.now(),
	}
	if opts != nil {
		entry.contentType = opts.ContentType
	}

	norm := normalizePath(path)
	m.files[norm] = entry
	m.fileNames[norm] = norm
	return buildStat(norm, entry, chooseSize(size, int64(len(data)))), nil
}

func (m *mockFS) download(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("mock r1fs: path is required")
	}

	m.mu.RLock()
	entry, ok := m.files[normalizePath(path)]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), entry.data...), nil
}

func (m *mockFS) addFile(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (*FileStat, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(filename) == "" {
		return nil, "", fmt.Errorf("mock r1fs: filename is required")
	}

	cid := newETag()
	stat, err := m.upload(ctx, cid, data, size, opts)
	if err != nil {
		return nil, "", err
	}

	m.mu.Lock()
	m.fileNames[cid] = filename
	m.mu.Unlock()
	stat.Path = cid
	return stat, cid, nil
}

func (m *mockFS) getFile(ctx context.Context, cid string) (*FileLocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("mock r1fs: cid is required")
	}

	norm := normalizePath(cid)
	m.mu.RLock()
	entry, ok := m.files[norm]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(norm, "/")
	}
	meta := make(map[string]any, len(entry.metadata)+2)
	meta["file"] = norm
	meta["filename"] = filename
	for k, v := range entry.metadata {
		meta[k] = v
	}
	return &FileLocation{
		Path:     norm,
		Filename: filename,
		Meta:     meta,
	}, nil
}

func (m *mockFS) addYAML(ctx context.Context, data any, filename string, secret string) (string, error) {
	if data == nil {
		return "", fmt.Errorf("mock r1fs: yaml data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: encode yaml data: %w", err)
	}
	if strings.TrimSpace(filename) == "" {
		filename = "document.yaml"
	}
	stat, cid, err := m.addFile(ctx, filename, payload, int64(len(payload)), &UploadOptions{Secret: secret})
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.yamlDocs[cid] = json.RawMessage(append([]byte(nil), payload...))
	m.mu.Unlock()
	return stat.Path, nil
}

func (m *mockFS) getYAML(ctx context.Context, cid string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("mock r1fs: cid is required")
	}

	m.mu.RLock()
	data, ok := m.yamlDocs[cid]
	m.mu.RUnlock()
	if !ok {
		return json.Marshal("error")
	}
	payload, err := json.Marshal(map[string]json.RawMessage{"file_data": data})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (m *mockFS) stat(ctx context.Context, path string) (*FileStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("mock r1fs: path is required")
	}

	m.mu.RLock()
	entry, ok := m.files[normalizePath(path)]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return buildStat(normalizePath(path), entry, int64(len(entry.data))), nil
}

func (m *mockFS) delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("mock r1fs: path is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	norm := normalizePath(path)
	if _, ok := m.files[norm]; !ok {
		return ErrNotFound
	}
	delete(m.files, norm)
	delete(m.fileNames, norm)
	delete(m.yamlDocs, norm)
	return nil
}

func (m *mockFS) list(ctx context.Context, dir string, cursor string, limit int) (*ListResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := normalizeDir(dir)

	m.mu.RLock()
	keys := make([]string, 0, len(m.files))
	for path := range m.files {
		if strings.HasPrefix(path, prefix) {
			keys = append(keys, path)
		}
	}
	m.mu.RUnlock()

	sort.Strings(keys)

	start := 0
	if cursor != "" {
		idx := sort.SearchStrings(keys, cursor)
		for idx < len(keys) && keys[idx] <= cursor {
			idx++
		}
		start = idx
	}
	if start > len(keys) {
		start = len(keys)
	}

	end := len(keys)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	files := make([]FileStat, 0, end-start)
	m.mu.RLock()
	for _, path := range keys[start:end] {
		entry := m.files[path]
		files = append(files, *buildStat(path, entry, int64(len(entry.data))))
	}
	m.mu.RUnlock()

	nextCursor := ""
	if end < len(keys) && end > 0 {
		nextCursor = keys[end-1]
	}

	return &ListResult{
		Files:      files,
		NextCursor: nextCursor,
	}, nil
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func normalizeDir(dir string) string {
	if dir == "" || dir == "/" {
		return "/"
	}
	s := normalizePath(dir)
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func optsMetadata(opts *UploadOptions) map[string]string {
	if opts == nil {
		return nil
	}
	meta := copyStringMap(opts.Metadata)
	if opts.Secret != "" {
		if meta == nil {
			meta = make(map[string]string)
		}
		meta["r1fs.secret"] = opts.Secret
	}
	return meta
}

func buildStat(path string, entry *fileEntry, size int64) *FileStat {
	var modPtr *time.Time
	if !entry.modTime.IsZero() {
		mod := entry.modTime
		modPtr = &mod
	}
	return &FileStat{
		Path:         path,
		Size:         size,
		ContentType:  entry.contentType,
		ETag:         entry.etag,
		LastModified: modPtr,
		Metadata:     copyStringMap(entry.metadata),
	}
}

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}
