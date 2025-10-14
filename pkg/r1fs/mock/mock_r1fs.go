package mock

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

type fileEntry struct {
	data     []byte
	metadata map[string]string
}

// Mock implements an in-memory filesystem for tests and sandboxing.
type Mock struct {
	mu        sync.RWMutex
	files     map[string]*fileEntry
	fileNames map[string]string
	yamlDocs  map[string]json.RawMessage
}

// New constructs an empty filesystem.
func New() *Mock {
	return &Mock{
		files:     make(map[string]*fileEntry),
		fileNames: make(map[string]string),
		yamlDocs:  make(map[string]json.RawMessage),
	}
}

// Seed loads files from seed entries.
func (m *Mock) Seed(entries []devseed.R1FSSeedEntry) error {
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
		m.files[path] = &fileEntry{
			data:     append([]byte(nil), data...),
			metadata: copyMap(e.Metadata),
		}
		m.fileNames[path] = path
	}
	return nil
}

// AddFileBase64 stores file contents via the base64 upload flow.
func (m *Mock) AddFileBase64(ctx context.Context, filename string, data io.Reader, size int64, opts *r1fs.UploadOptions) (cid string, err error) {
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("mock r1fs: filename is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	payload, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: read payload: %w", err)
	}

	return m.AddFile(ctx, filename, payload, size, opts)
}

// AddFile stores contents using a generated CID, mimicking /add_file behaviour.
func (m *Mock) AddFile(ctx context.Context, filename string, data []byte, _ int64, opts *r1fs.UploadOptions) (cid string, err error) {
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("mock r1fs: filename is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	entry := &fileEntry{
		data:     append([]byte(nil), data...),
		metadata: optsMetadata(opts),
	}
	if entry.metadata == nil {
		entry.metadata = make(map[string]string)
	}
	entry.metadata["r1fs.filename"] = filename

	cid = newETag()
	norm := normalizePath(cid)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[norm] = entry
	if m.fileNames == nil {
		m.fileNames = make(map[string]string)
	}
	if _, exists := m.fileNames[norm]; !exists {
		m.fileNames[norm] = norm
	}
	m.fileNames[cid] = filename

	return cid, nil
}

// GetFileBase64 returns stored file contents and filename.
func (m *Mock) GetFileBase64(ctx context.Context, cid string, _ string) (fileData []byte, fileName string, err error) {
	if strings.TrimSpace(cid) == "" {
		return nil, "", fmt.Errorf("mock r1fs: cid is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	m.mu.RLock()
	norm := normalizePath(cid)
	entry, ok := m.files[norm]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, "", r1fs.ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(norm, "/")
	}

	return append([]byte(nil), entry.data...), filename, nil
}

// GetFile resolves metadata for a stored CID.
func (m *Mock) GetFile(ctx context.Context, cid string, _ string) (location *r1fs.FileLocation, err error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("mock r1fs: cid is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	normalized := normalizePath(cid)
	m.mu.RLock()
	entry, ok := m.files[normalized]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, r1fs.ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(normalized, "/")
	}
	meta := make(map[string]any, len(entry.metadata)+2)
	meta["file"] = normalized
	meta["filename"] = filename
	for k, v := range entry.metadata {
		meta[k] = v
	}
	return &r1fs.FileLocation{
		Path:     normalized,
		Filename: filename,
		Meta:     meta,
	}, nil
}

// AddYAML stores structured data and returns a CID.
func (m *Mock) AddYAML(ctx context.Context, data any, filename string, secret string) (cid string, err error) {
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
	cid, err = m.AddFile(ctx, filename, payload, int64(len(payload)), &r1fs.UploadOptions{Secret: secret})
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.yamlDocs[cid] = json.RawMessage(append([]byte(nil), payload...))
	m.mu.Unlock()
	return cid, nil
}

// GetYAML retrieves YAML data previously stored with AddYAML.
func (m *Mock) GetYAML(ctx context.Context, cid string, _ string) (payload []byte, err error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("mock r1fs: cid is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	data, ok := m.yamlDocs[cid]
	m.mu.RUnlock()
	if !ok {
		return json.Marshal("error")
	}
	payload, err = json.Marshal(map[string]json.RawMessage{"file_data": data})
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

func copyMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func optsMetadata(opts *r1fs.UploadOptions) map[string]string {
	if opts == nil {
		return nil
	}
	meta := copyMap(opts.Metadata)
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
