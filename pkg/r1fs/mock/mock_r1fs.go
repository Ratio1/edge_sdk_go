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
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

type fileEntry struct {
	data        []byte
	contentType string
	etag        string
	metadata    map[string]string
	modTime     time.Time
}

// Mock implements an in-memory filesystem for tests and sandboxing.
type Mock struct {
	mu        sync.RWMutex
	files     map[string]*fileEntry
	now       func() time.Time
	fileNames map[string]string
	yamlDocs  map[string]json.RawMessage
}

// Option configures the mock filesystem.
type Option func(*Mock)

// WithClock overrides the time source.
func WithClock(fn func() time.Time) Option {
	return func(m *Mock) {
		if fn != nil {
			m.now = fn
		}
	}
}

// New constructs an empty filesystem.
func New(opts ...Option) *Mock {
	m := &Mock{
		files:     make(map[string]*fileEntry),
		fileNames: make(map[string]string),
		yamlDocs:  make(map[string]json.RawMessage),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Mock) clock() time.Time {
	if m.now == nil {
		return time.Now().UTC()
	}
	return m.now()
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
		meta := copyMap(e.Metadata)
		entry := &fileEntry{
			data:        data,
			contentType: e.ContentType,
			etag:        newETag(),
			metadata:    meta,
			modTime:     m.clock(),
		}
		if e.LastModified != nil {
			entry.modTime = e.LastModified.UTC()
		}
		m.files[path] = entry
		if m.fileNames != nil {
			key := strings.TrimPrefix(path, "/")
			if key == "" {
				key = "/"
			}
			m.fileNames[key] = path
		}
	}
	return nil
}

// AddFileBase64 stores file contents via the base64 upload flow.
func (m *Mock) AddFileBase64(ctx context.Context, filename string, data io.Reader, size int64, opts *r1fs.UploadOptions) (*r1fs.FileStat, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("mock r1fs: filename is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("mock r1fs: read payload: %w", err)
	}

	return m.AddFile(ctx, filename, payload, size, opts)
}

// AddFile stores contents using a generated CID, mimicking /add_file behaviour.
func (m *Mock) AddFile(ctx context.Context, filename string, data []byte, size int64, opts *r1fs.UploadOptions) (*r1fs.FileStat, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("mock r1fs: filename is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock()
	meta := copyMap(optsMetadata(opts))
	if meta == nil {
		meta = make(map[string]string)
	}
	meta["r1fs.filename"] = filename

	entry := &fileEntry{
		data:        append([]byte(nil), data...),
		contentType: "",
		etag:        newETag(),
		metadata:    meta,
		modTime:     now,
	}
	if opts != nil {
		entry.contentType = opts.ContentType
	}

	cid := newETag()
	path := normalizePath(cid)
	m.files[path] = entry
	if m.fileNames == nil {
		m.fileNames = make(map[string]string)
	}
	m.fileNames[cid] = filename

	stat := buildStat(path, entry, chooseSize(size, int64(len(data))))
	stat.Path = cid
	return stat, nil
}

// GetFileBase64 returns stored file contents as bytes.
func (m *Mock) GetFileBase64(ctx context.Context, cid string, _ string) ([]byte, error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("mock r1fs: cid is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	entry, ok := m.files[normalizePath(cid)]
	m.mu.RUnlock()
	if !ok {
		return nil, r1fs.ErrNotFound
	}

	return append([]byte(nil), entry.data...), nil
}

// GetFile resolves metadata for a stored CID.
func (m *Mock) GetFile(ctx context.Context, cid string, secret string) (*r1fs.FileLocation, error) {
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
func (m *Mock) AddYAML(ctx context.Context, data any, filename string, secret string) (string, error) {
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
	stat, err := m.AddFile(ctx, filename, payload, int64(len(payload)), &r1fs.UploadOptions{Secret: secret})
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.yamlDocs[stat.Path] = json.RawMessage(append([]byte(nil), payload...))
	m.mu.Unlock()
	return stat.Path, nil
}

// GetYAML retrieves YAML data previously stored with AddYAML.
func (m *Mock) GetYAML(ctx context.Context, cid string, secret string) ([]byte, error) {
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
	payload, err := json.Marshal(map[string]json.RawMessage{"file_data": data})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func buildStat(path string, entry *fileEntry, size int64) *r1fs.FileStat {
	var modPtr *time.Time
	if !entry.modTime.IsZero() {
		mod := entry.modTime
		modPtr = &mod
	}
	metadata := copyMap(entry.metadata)
	return &r1fs.FileStat{
		Path:         path,
		Size:         size,
		ContentType:  entry.contentType,
		ETag:         entry.etag,
		LastModified: modPtr,
		Metadata:     metadata,
	}
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

func chooseSize(provided int64, actual int64) int64 {
	if provided >= 0 {
		return provided
	}
	return actual
}

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}
