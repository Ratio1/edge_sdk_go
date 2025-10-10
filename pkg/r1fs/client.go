package r1fs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/httpx"
	"github.com/Ratio1/ratio1_sdk_go/internal/ratio1api"
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

// AddFileBase64 writes data via /add_file_base64. The returned FileStat uses the CID
// reported by the upstream API as its Path field.
func (c *Client) AddFileBase64(ctx context.Context, filename string, data io.Reader, size int64, opts *UploadOptions) (*FileStat, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("r1fs: filename is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("r1fs: read upload payload: %w", err)
	}
	return c.backend.AddFileBase64(ctx, filename, payload, size, opts)
}

// AddFile uploads data using the /add_file endpoint (multipart form upload).
func (c *Client) AddFile(ctx context.Context, filename string, data io.Reader, size int64, opts *UploadOptions) (*FileStat, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("r1fs: filename is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("r1fs: read upload payload: %w", err)
	}
	return c.backend.AddFile(ctx, filename, payload, size, opts)
}

// GetFileBase64 retrieves and decodes data via /get_file_base64, returning the upstream filename.
func (c *Client) GetFileBase64(ctx context.Context, cid string, secret string) ([]byte, string, error) {
	if strings.TrimSpace(cid) == "" {
		return nil, "", fmt.Errorf("r1fs: cid is required")
	}
	if c == nil || c.backend == nil {
		return nil, "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.GetFileBase64(ctx, cid, secret)
}

// GetFile resolves a CID to the on-disk path reported by /get_file.
func (c *Client) GetFile(ctx context.Context, cid string, secret string) (*FileLocation, error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("r1fs: cid is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.GetFile(ctx, cid, secret)
}

// AddYAML stores structured data as YAML via /add_yaml and returns the assigned CID.
func (c *Client) AddYAML(ctx context.Context, data any, opts *YAMLOptions) (string, error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	var filename, secret string
	if opts != nil {
		filename = opts.Filename
		secret = opts.Secret
	}
	return c.backend.AddYAML(ctx, data, filename, secret)
}

// GetYAML retrieves YAML content as raw JSON. Provide out to decode into a struct.
func (c *Client) GetYAML(ctx context.Context, cid string, secret string, out any) (*YAMLDocument[json.RawMessage], error) {
	if c == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	data, err := c.getYAMLRaw(ctx, cid, secret)
	if err != nil {
		return nil, err
	}
	doc, err := decodeYAMLDocument[json.RawMessage](cid, data)
	if err != nil || doc == nil || out == nil {
		return doc, err
	}
	if len(doc.Data) == 0 {
		return doc, nil
	}
	if err := json.Unmarshal(doc.Data, out); err != nil {
		return nil, fmt.Errorf("r1fs: decode YAML payload: %w", err)
	}
	return doc, nil
}

func (c *Client) getYAMLRaw(ctx context.Context, cid string, secret string) ([]byte, error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("r1fs: cid is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.GetYAML(ctx, cid, secret)
}

func chooseSize(provided int64, actual int64) int64 {
	if provided >= 0 {
		return provided
	}
	return actual
}

func cloneMeta(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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

func decodeYAMLDocument[T any](cid string, data []byte) (*YAMLDocument[T], error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var str string
	if err := json.Unmarshal(trimmed, &str); err == nil {
		if strings.EqualFold(str, "error") {
			return nil, fmt.Errorf("r1fs: get_yaml reported error for cid %s", cid)
		}
	}

	var payload struct {
		FileData json.RawMessage `json:"file_data"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("r1fs: decode get_yaml response: %w", err)
	}

	if len(payload.FileData) == 0 {
		return &YAMLDocument[T]{CID: cid}, nil
	}

	var value T
	if err := json.Unmarshal(payload.FileData, &value); err != nil {
		return nil, fmt.Errorf("r1fs: decode YAML payload: %w", err)
	}
	return &YAMLDocument[T]{CID: cid, Data: value}, nil
}

type Backend interface {
	AddFileBase64(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (*FileStat, error)
	AddFile(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (*FileStat, error)
	GetFileBase64(ctx context.Context, cid string, secret string) ([]byte, string, error)
	GetFile(ctx context.Context, cid string, secret string) (*FileLocation, error)
	AddYAML(ctx context.Context, data any, filename string, secret string) (string, error)
	GetYAML(ctx context.Context, cid string, secret string) ([]byte, error)
}

type httpBackend struct {
	client *httpx.Client
}

func (b *httpBackend) AddFileBase64(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (*FileStat, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"file_base64_str": base64.StdEncoding.EncodeToString(data),
		"filename":        filename,
	}
	if opts != nil {
		if opts.Secret != "" {
			body["secret"] = opts.Secret
		}
		if len(opts.Metadata) > 0 {
			body["metadata"] = opts.Metadata
		}
		if opts.ContentType != "" {
			body["content_type"] = opts.ContentType
		}
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
	var response struct {
		CID string `json:"cid"`
	}
	if err := ratio1api.DecodeResult(payloadBytes, &response); err != nil {
		return nil, fmt.Errorf("r1fs: decode add_file_base64 response: %w", err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return nil, fmt.Errorf("r1fs: missing cid in response")
	}
	stat := &FileStat{
		Path:        response.CID,
		Size:        chooseSize(size, int64(len(data))),
		ContentType: "",
		Metadata:    copyMap(opts),
	}
	if opts != nil {
		stat.ContentType = opts.ContentType
	}
	return stat, nil
}

func (b *httpBackend) AddFile(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (*FileStat, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("r1fs: create multipart part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return nil, fmt.Errorf("r1fs: write multipart payload: %w", err)
	}
	meta := make(map[string]any)
	if opts != nil {
		if opts.Secret != "" {
			meta["secret"] = opts.Secret
		}
		if len(opts.Metadata) > 0 {
			meta["metadata"] = opts.Metadata
		}
		if opts.ContentType != "" {
			meta["content_type"] = opts.ContentType
		}
	}
	if len(meta) == 0 {
		meta = map[string]any{}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("r1fs: encode multipart metadata: %w", err)
	}
	if err := writer.WriteField("body_json", string(metaBytes)); err != nil {
		return nil, fmt.Errorf("r1fs: write multipart metadata: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("r1fs: finalize multipart body: %w", err)
	}
	payload := body.Bytes()
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "add_file",
		Header: http.Header{"Content-Type": []string{writer.FormDataContentType()}},
		Body:   bytes.NewReader(payload),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
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
	var response struct {
		CID string `json:"cid"`
	}
	if err := ratio1api.DecodeResult(payloadBytes, &response); err != nil {
		return nil, fmt.Errorf("r1fs: decode add_file response: %w", err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return nil, fmt.Errorf("r1fs: missing cid in add_file response")
	}
	stat := &FileStat{
		Path:        response.CID,
		Size:        chooseSize(size, int64(len(data))),
		ContentType: "",
		Metadata:    copyMap(opts),
	}
	if opts != nil {
		stat.ContentType = opts.ContentType
	}
	return stat, nil
}

func (b *httpBackend) GetFileBase64(ctx context.Context, cid string, secret string) ([]byte, string, error) {
	if b == nil || b.client == nil {
		return nil, "", fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"cid": cid,
	}
	if strings.TrimSpace(secret) != "" {
		body["secret"] = secret
	}
	jsonBody, err := encodeJSON(body)
	if err != nil {
		return nil, "", err
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
		return nil, "", err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, "", err
	}
	var result struct {
		FileBase64 string `json:"file_base64_str"`
		Filename   string `json:"filename"`
	}
	if err := ratio1api.DecodeResult(payloadBytes, &result); err != nil {
		return nil, "", fmt.Errorf("r1fs: decode get_file_base64 response: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(result.FileBase64)
	if err != nil {
		return nil, "", fmt.Errorf("r1fs: decode base64 payload: %w", err)
	}
	return data, result.Filename, nil
}

func (b *httpBackend) GetFile(ctx context.Context, cid string, secret string) (*FileLocation, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	query := url.Values{"cid": {cid}}
	if strings.TrimSpace(secret) != "" {
		query.Set("secret", secret)
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "get_file",
		Query:  query,
	})
	if err != nil {
		return nil, err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		FilePath string         `json:"file_path"`
		Meta     map[string]any `json:"meta"`
	}
	if err := ratio1api.DecodeResult(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("r1fs: decode get_file response: %w", err)
	}
	loc := &FileLocation{
		Path: payload.FilePath,
		Meta: cloneMeta(payload.Meta),
	}
	if loc.Meta != nil {
		if name, ok := loc.Meta["filename"].(string); ok {
			loc.Filename = name
		}
	}
	if loc.Filename == "" && payload.FilePath != "" {
		parts := strings.Split(payload.FilePath, "/")
		loc.Filename = parts[len(parts)-1]
	}
	return loc, nil
}

func (b *httpBackend) AddYAML(ctx context.Context, data any, filename string, secret string) (string, error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"data": data,
	}
	if strings.TrimSpace(filename) != "" {
		body["fn"] = filename
	}
	if strings.TrimSpace(secret) != "" {
		body["secret"] = secret
	}
	jsonBody, err := encodeJSON(body)
	if err != nil {
		return "", err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "add_yaml",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   bytes.NewReader(jsonBody),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(jsonBody)), nil
		},
	}
	resp, err := b.client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return "", err
	}
	var response struct {
		CID string `json:"cid"`
	}
	if err := ratio1api.DecodeResult(payloadBytes, &response); err != nil {
		return "", fmt.Errorf("r1fs: decode add_yaml response: %w", err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return "", fmt.Errorf("r1fs: missing cid in add_yaml response")
	}
	return response.CID, nil
}

func (b *httpBackend) GetYAML(ctx context.Context, cid string, secret string) ([]byte, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("r1fs: http backend not configured")
	}
	query := url.Values{"cid": {cid}}
	if strings.TrimSpace(secret) != "" {
		query.Set("secret", secret)
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "get_yaml",
		Query:  query,
	})
	if err != nil {
		return nil, err
	}
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	data, err := ratio1api.ExtractResult(payloadBytes)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return data, nil
}
