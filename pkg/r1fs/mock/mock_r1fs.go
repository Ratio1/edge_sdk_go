package mock

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/gob"
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
	data []byte
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
			data: append([]byte(nil), data...),
		}
		m.fileNames[path] = path
	}
	return nil
}

// AddFileBase64 stores file contents via the base64 upload flow.
func (m *Mock) AddFileBase64(ctx context.Context, data io.Reader, opts *r1fs.DataOptions) (cid string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: read payload: %w", err)
	}
	return m.AddFile(ctx, bytes.NewReader(payload), opts)
}

// AddFile stores contents using a generated CID, mimicking /add_file behaviour.
func (m *Mock) AddFile(ctx context.Context, data io.Reader, opts *r1fs.DataOptions) (cid string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: read payload: %w", err)
	}
	name := preferredName(opts)
	if name == "" {
		return "", fmt.Errorf("mock r1fs: filename or filepath is required")
	}

	entry := &fileEntry{data: append([]byte(nil), payload...)}
	cid = newETag()
	norm := normalizePath(cid)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = make(map[string]*fileEntry)
	}
	if m.fileNames == nil {
		m.fileNames = make(map[string]string)
	}
	m.files[norm] = entry
	m.fileNames[cid] = name

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
	entry, ok := m.files[normalizePath(cid)]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, "", r1fs.ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(cid, "/")
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
	_, ok := m.files[normalized]
	filename := m.fileNames[cid]
	m.mu.RUnlock()
	if !ok {
		return nil, r1fs.ErrNotFound
	}
	if filename == "" {
		filename = strings.TrimPrefix(normalized, "/")
	}
	meta := map[string]any{
		"file":     normalized,
		"filename": filename,
	}
	return &r1fs.FileLocation{
		Path:     normalized,
		Filename: filename,
		Meta:     meta,
	}, nil
}

// Status returns a snapshot of stored entries.
func (m *Mock) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	files := make([]string, 0, len(m.fileNames))
	for cid := range m.fileNames {
		files = append(files, cid)
	}
	return map[string]any{
		"files": files,
		"count": len(files),
	}
}

// AddJSON stores JSON data and returns a CID.
func (m *Mock) AddJSON(ctx context.Context, data any, opts *r1fs.DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("mock r1fs: json data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: encode json data: %w", err)
	}
	return m.AddFile(ctx, bytes.NewReader(payload), opts)
}

// AddPickle serialises data to pickle and stores it.
func (m *Mock) AddPickle(ctx context.Context, data any, opts *r1fs.DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("mock r1fs: pickle data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return "", fmt.Errorf("mock r1fs: encode pickle data: %w", err)
	}
	return m.AddFile(ctx, &buf, opts)
}

// CalculateJSONCID returns a deterministic CID for JSON data.
func (m *Mock) CalculateJSONCID(ctx context.Context, data any, nonce int, opts *r1fs.DataOptions) (string, error) {
	if data == nil {
		return "", fmt.Errorf("mock r1fs: json data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("mock r1fs: encode json data: %w", err)
	}
	return deterministicCID(payload, nonce, opts), nil
}

// CalculatePickleCID returns a deterministic CID for pickle data.
func (m *Mock) CalculatePickleCID(ctx context.Context, data any, nonce int, opts *r1fs.DataOptions) (string, error) {
	if data == nil {
		return "", fmt.Errorf("mock r1fs: pickle data is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return "", fmt.Errorf("mock r1fs: encode pickle data: %w", err)
	}
	return deterministicCID(buf.Bytes(), nonce, opts), nil
}

// AddYAML stores structured data and returns a CID.
func (m *Mock) AddYAML(ctx context.Context, data any, opts *r1fs.DataOptions) (cid string, err error) {
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
	cid, err = m.AddFile(ctx, bytes.NewReader(payload), opts)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	if m.yamlDocs == nil {
		m.yamlDocs = make(map[string]json.RawMessage)
	}
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

func preferredName(opts *r1fs.DataOptions) string {
	if opts == nil {
		return ""
	}
	if path := strings.TrimSpace(opts.FilePath); path != "" {
		return path
	}
	return strings.TrimSpace(opts.Filename)
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

func deterministicCID(payload []byte, nonce int, opts *r1fs.DataOptions) string {
	hash := sha256.New()
	hash.Write(payload)
	var nonceBuf [8]byte
	binary.LittleEndian.PutUint64(nonceBuf[:], uint64(nonce))
	hash.Write(nonceBuf[:])
	if opts != nil {
		if strings.TrimSpace(opts.Secret) != "" {
			hash.Write([]byte(opts.Secret))
		}
		if strings.TrimSpace(opts.Filename) != "" {
			hash.Write([]byte(opts.Filename))
		}
		if strings.TrimSpace(opts.FilePath) != "" {
			hash.Write([]byte(opts.FilePath))
		}
		if opts.Nonce != nil {
			var pointerNonce [8]byte
			binary.LittleEndian.PutUint64(pointerNonce[:], uint64(*opts.Nonce))
			hash.Write(pointerNonce[:])
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}
