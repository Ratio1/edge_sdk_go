package r1fs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/httpx"
)

// Client provides HTTP access to the R1FS manager API.
type Client struct {
	backend Backend
}

// New constructs an HTTP-backed client.
func New(baseURL string, opts ...httpx.Option) (*Client, error) {
	cl, err := httpx.NewClient(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	return NewWithHTTPClient(cl), nil
}

// NewWithHTTPClient wraps an existing httpx.Client.
func NewWithHTTPClient(httpClient *httpx.Client) *Client {
	return &Client{backend: &httpBackend{client: httpClient}}
}

// NewWithBackend allows callers to provide a custom backend (e.g., mocks).
func NewWithBackend(b Backend) *Client {
	return &Client{backend: b}
}

// Upload writes data via /add_file_base64. The returned FileStat uses the CID
// reported by the upstream API as its Path field.
func (c *Client) Upload(ctx context.Context, path string, data io.Reader, size int64, opts *UploadOptions) (*FileStat, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("r1fs: path is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("r1fs: read upload payload: %w", err)
	}
	return c.backend.Upload(ctx, path, payload, size, opts)
}

// Download retrieves data via /get_file_base64 and streams decoded bytes into w.
func (c *Client) Download(ctx context.Context, path string, w io.Writer) (int64, error) {
	if strings.TrimSpace(path) == "" {
		return 0, fmt.Errorf("r1fs: path is required")
	}
	if c == nil || c.backend == nil {
		return 0, fmt.Errorf("r1fs: client is nil")
	}
	data, err := c.backend.Download(ctx, path)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// Stat is currently not exposed by the upstream API.
func (c *Client) Stat(ctx context.Context, path string) (*FileStat, error) {
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.Stat(ctx, path)
}

// List is currently not exposed by the upstream API.
func (c *Client) List(ctx context.Context, dir string, cursor string, limit int) (*ListResult, error) {
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.List(ctx, dir, cursor, limit)
}

// Delete is currently not exposed by the upstream API.
func (c *Client) Delete(ctx context.Context, path string) error {
	if c == nil || c.backend == nil {
		return fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.Delete(ctx, path)
}

func chooseSize(provided int64, actual int64) int64 {
	if provided >= 0 {
		return provided
	}
	return actual
}

func encodeJSON(payload any) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func copyMap(opts *UploadOptions) map[string]string {
	if opts == nil || len(opts.Metadata) == 0 {
		return nil
	}
	dst := make(map[string]string, len(opts.Metadata))
	for k, v := range opts.Metadata {
		dst[k] = v
	}
	if opts.Secret != "" {
		dst["r1fs.secret"] = opts.Secret
	}
	return dst
}

type Backend interface {
	Upload(ctx context.Context, path string, data []byte, size int64, opts *UploadOptions) (*FileStat, error)
	Download(ctx context.Context, path string) ([]byte, error)
	Stat(ctx context.Context, path string) (*FileStat, error)
	List(ctx context.Context, dir string, cursor string, limit int) (*ListResult, error)
	Delete(ctx context.Context, path string) error
}

type httpBackend struct {
	client *httpx.Client
}

func (b *httpBackend) Upload(ctx context.Context, path string, data []byte, size int64, opts *UploadOptions) (*FileStat, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"file_base64_str": base64.StdEncoding.EncodeToString(data),
		"filename":        filepath.Base(path),
	}
	if opts != nil && opts.Secret != "" {
		body["secret"] = opts.Secret
	}
	jsonBody, err := encodeJSON(body)
	if err != nil {
		return nil, err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "add_file_base64",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   bytes.NewReader(jsonBody),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(jsonBody)), nil
		},
	}
	resp, err := b.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	var result struct {
		CID string `json:"cid"`
	}
	if err := json.Unmarshal(payloadBytes, &result); err != nil {
		return nil, fmt.Errorf("r1fs: decode add_file_base64 response: %w", err)
	}
	if strings.TrimSpace(result.CID) == "" {
		return nil, fmt.Errorf("r1fs: missing cid in response")
	}
	stat := &FileStat{
		Path:        result.CID,
		Size:        chooseSize(size, int64(len(data))),
		ContentType: "",
		Metadata:    copyMap(opts),
	}
	if opts != nil {
		stat.ContentType = opts.ContentType
	}
	return stat, nil
}

func (b *httpBackend) Download(ctx context.Context, path string) ([]byte, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"cid": path,
	}
	jsonBody, err := encodeJSON(body)
	if err != nil {
		return nil, err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "get_file_base64",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   bytes.NewReader(jsonBody),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(jsonBody)), nil
		},
	}
	resp, err := b.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	var result struct {
		FileBase64 string `json:"file_base64_str"`
	}
	if err := json.Unmarshal(payloadBytes, &result); err != nil {
		return nil, fmt.Errorf("r1fs: decode get_file_base64 response: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(result.FileBase64)
	if err != nil {
		return nil, fmt.Errorf("r1fs: decode base64 payload: %w", err)
	}
	return data, nil
}

func (b *httpBackend) Stat(ctx context.Context, path string) (*FileStat, error) {
	return nil, fmt.Errorf("%w: stat not available in r1fs_manager_api.py", ErrUnsupportedFeature)
}

func (b *httpBackend) List(ctx context.Context, dir string, cursor string, limit int) (*ListResult, error) {
	return nil, fmt.Errorf("%w: list not available in r1fs_manager_api.py", ErrUnsupportedFeature)
}

func (b *httpBackend) Delete(ctx context.Context, path string) error {
	return fmt.Errorf("%w: delete not available in r1fs_manager_api.py", ErrUnsupportedFeature)
}
