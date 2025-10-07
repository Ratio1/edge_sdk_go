package mock

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
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
	mu    sync.RWMutex
	files map[string]*fileEntry
	now   func() time.Time
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
		files: make(map[string]*fileEntry),
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
	}
	return nil
}

// Upload stores file contents.
func (m *Mock) Upload(ctx context.Context, path string, data io.Reader, size int64, opts *r1fs.UploadOptions) (*r1fs.FileStat, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("mock r1fs: path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("mock r1fs: read payload: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock()
	entry := &fileEntry{
		data:        append([]byte(nil), payload...),
		contentType: "",
		etag:        newETag(),
		metadata:    copyMap(optsMetadata(opts)),
		modTime:     now,
	}
	if opts != nil {
		entry.contentType = opts.ContentType
	}

	normalized := normalizePath(path)
	m.files[normalized] = entry

	return buildStat(normalized, entry, chooseSize(size, int64(len(payload)))), nil
}

// Download streams file contents into w.
func (m *Mock) Download(ctx context.Context, path string, w io.Writer) (int64, error) {
	if strings.TrimSpace(path) == "" {
		return 0, fmt.Errorf("mock r1fs: path is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	m.mu.RLock()
	entry, ok := m.files[normalizePath(path)]
	m.mu.RUnlock()
	if !ok {
		return 0, r1fs.ErrNotFound
	}

	n, err := w.Write(entry.data)
	return int64(n), err
}

// Stat returns metadata for the given file.
func (m *Mock) Stat(ctx context.Context, path string) (*r1fs.FileStat, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("mock r1fs: path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	entry, ok := m.files[normalizePath(path)]
	m.mu.RUnlock()
	if !ok {
		return nil, r1fs.ErrNotFound
	}
	return buildStat(normalizePath(path), entry, int64(len(entry.data))), nil
}

// Delete removes a file.
func (m *Mock) Delete(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("mock r1fs: path is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[normalizePath(path)]; !ok {
		return r1fs.ErrNotFound
	}
	delete(m.files, normalizePath(path))
	return nil
}

// List enumerates files under dir using lexical ordering.
func (m *Mock) List(ctx context.Context, dir string, cursor string, limit int) (*r1fs.ListResult, error) {
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

	files := make([]r1fs.FileStat, 0, end-start)
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

	return &r1fs.ListResult{
		Files:      files,
		NextCursor: nextCursor,
	}, nil
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
