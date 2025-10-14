package r1fs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
)

type mockFS struct {
	mu        sync.RWMutex
	files     map[string]*fileEntry
	fileNames map[string]string
	yamlDocs  map[string]json.RawMessage
}

type fileEntry struct {
	data     []byte
	metadata map[string]string
}

func newMockFS() *mockFS {
	return &mockFS{
		files:     make(map[string]*fileEntry),
		fileNames: make(map[string]string),
		yamlDocs:  make(map[string]json.RawMessage),
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
			data:     append([]byte(nil), data...),
			metadata: copyStringMap(e.Metadata),
		}
		m.files[path] = entry
		m.fileNames[path] = path
	}
	return nil
}

func (m *mockFS) addFileBase64(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (string, error) {
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("mock r1fs: filename is required")
	}
	return m.addFile(ctx, filename, data, size, opts)
}

func (m *mockFS) upload(ctx context.Context, path string, data []byte, opts *UploadOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("mock r1fs: path is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &fileEntry{
		data:     append([]byte(nil), data...),
		metadata: optsMetadata(opts),
	}

	norm := normalizePath(path)
	m.files[norm] = entry
	m.fileNames[norm] = norm
	return norm, nil
}

func (m *mockFS) getFileBase64(ctx context.Context, cid string, _ string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(cid) == "" {
		return nil, "", fmt.Errorf("mock r1fs: cid is required")
	}

	m.mu.RLock()
	norm := normalizePath(cid)
	entry, ok := m.files[norm]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, "", ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(norm, "/")
	}
	return append([]byte(nil), entry.data...), filename, nil
}

func (m *mockFS) addFile(ctx context.Context, filename string, data []byte, _ int64, opts *UploadOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("mock r1fs: filename is required")
	}

	cid := newETag()
	if _, err := m.upload(ctx, cid, data, opts); err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.fileNames[cid] = filename
	return cid, nil
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
	cid, err := m.addFile(ctx, filename, payload, int64(len(payload)), &UploadOptions{Secret: secret})
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.yamlDocs[cid] = json.RawMessage(append([]byte(nil), payload...))
	m.mu.Unlock()
	return cid, nil
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

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
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

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}
