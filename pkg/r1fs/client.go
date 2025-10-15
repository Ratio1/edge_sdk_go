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
	"path/filepath"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/httpx"
	"github.com/Ratio1/ratio1_sdk_go/internal/ratio1api"
)

// Client provides HTTP access to the R1FS manager API.
type Client struct {
	backend Backend
}

// New constructs an HTTP-backed client.
func New(baseURL string, opts ...httpx.Option) (client *Client, err error) {
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

// AddFileBase64 writes data via /add_file_base64 and returns the upstream CID.
func (c *Client) AddFileBase64(ctx context.Context, data io.Reader, opts *DataOptions) (cid string, err error) {
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("r1fs: read upload payload: %w", err)
	}
	return c.backend.AddFileBase64(ctx, payload, opts)
}

// AddFile uploads data using the /add_file endpoint (multipart form upload) and returns the upstream CID.
func (c *Client) AddFile(ctx context.Context, data io.Reader, opts *DataOptions) (cid string, err error) {
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	payload, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("r1fs: read upload payload: %w", err)
	}
	return c.backend.AddFile(ctx, payload, opts)
}

// GetFileBase64 retrieves and decodes data via /get_file_base64, returning the upstream filename.
func (c *Client) GetFileBase64(ctx context.Context, cid string, secret string) (fileData []byte, fileName string, err error) {
	if strings.TrimSpace(cid) == "" {
		return nil, "", fmt.Errorf("r1fs: cid is required")
	}
	if c == nil || c.backend == nil {
		return nil, "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.GetFileBase64(ctx, cid, secret)
}

// GetFile resolves a CID to the on-disk path reported by /get_file.
func (c *Client) GetFile(ctx context.Context, cid string, secret string) (location *FileLocation, err error) {
	if strings.TrimSpace(cid) == "" {
		return nil, fmt.Errorf("r1fs: cid is required")
	}
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.GetFile(ctx, cid, secret)
}

// AddJSON stores structured JSON data via /add_json and returns the upstream CID.
func (c *Client) AddJSON(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.AddJSON(ctx, data, opts)
}

// AddPickle serialises data to pickle via /add_pickle and returns the upstream CID.
func (c *Client) AddPickle(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.AddPickle(ctx, data, opts)
}

// CalculateJSONCID deterministically calculates the CID for JSON data without storing it.
func (c *Client) CalculateJSONCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.CalculateJSONCID(ctx, data, nonce, opts)
}

// CalculatePickleCID deterministically calculates the CID for pickle data without storing it.
func (c *Client) CalculatePickleCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.CalculatePickleCID(ctx, data, nonce, opts)
}

// AddYAML stores structured data as YAML via /add_yaml and returns the assigned CID.
func (c *Client) AddYAML(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if data == nil {
		return "", fmt.Errorf("r1fs: data is required")
	}
	if c == nil || c.backend == nil {
		return "", fmt.Errorf("r1fs: client is nil")
	}
	return c.backend.AddYAML(ctx, data, opts)
}

// GetYAML retrieves YAML content as raw JSON. Provide out to decode into a struct.
func (c *Client) GetYAML(ctx context.Context, cid string, secret string, out any) (doc *YAMLDocument[json.RawMessage], err error) {
	if c == nil {
		return nil, fmt.Errorf("r1fs: client is nil")
	}
	data, err := c.getYAMLRaw(ctx, cid, secret)
	if err != nil {
		return nil, err
	}
	doc, err = decodeYAMLDocument[json.RawMessage](cid, data)
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
	AddFileBase64(ctx context.Context, data []byte, opts *DataOptions) (cid string, err error)
	AddFile(ctx context.Context, data []byte, opts *DataOptions) (cid string, err error)
	GetFileBase64(ctx context.Context, cid string, secret string) (fileData []byte, fileName string, err error)
	GetFile(ctx context.Context, cid string, secret string) (location *FileLocation, err error)
	AddJSON(ctx context.Context, data any, opts *DataOptions) (cid string, err error)
	AddPickle(ctx context.Context, data any, opts *DataOptions) (cid string, err error)
	CalculateJSONCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error)
	CalculatePickleCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error)
	AddYAML(ctx context.Context, data any, opts *DataOptions) (cid string, err error)
	GetYAML(ctx context.Context, cid string, secret string) (payload []byte, err error)
}

type httpBackend struct {
	client *httpx.Client
}

func (b *httpBackend) AddFileBase64(ctx context.Context, data []byte, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"file_base64_str": base64.StdEncoding.EncodeToString(data),
	}
	if opts != nil {
		applyPathOptions(body, opts)
	}
	if _, ok := body["filename"]; !ok {
		return "", fmt.Errorf("r1fs: filename or filepath is required")
	}
	jsonBody, err := encodeJSON(body)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("r1fs: decode add_file_base64 response: %w", err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return "", fmt.Errorf("r1fs: missing cid in response")
	}
	return response.CID, nil
}

func (b *httpBackend) AddFile(ctx context.Context, data []byte, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	filename := resolveUploadName(opts)
	if strings.TrimSpace(filename) == "" {
		return "", fmt.Errorf("r1fs: filename or filepath is required")
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("r1fs: create multipart part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return "", fmt.Errorf("r1fs: write multipart payload: %w", err)
	}
	meta := applyBodyJSON(opts)
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("r1fs: encode multipart metadata: %w", err)
	}
	if err := writer.WriteField("body_json", string(metaBytes)); err != nil {
		return "", fmt.Errorf("r1fs: write multipart metadata: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("r1fs: finalize multipart body: %w", err)
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
		return "", fmt.Errorf("r1fs: decode add_file response: %w", err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return "", fmt.Errorf("r1fs: missing cid in add_file response")
	}
	return response.CID, nil
}

func (b *httpBackend) AddJSON(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	payload := map[string]any{
		"data": data,
	}
	applyDataOptions(payload, opts)
	return b.postCIDRequest(ctx, "add_json", payload, "add_json")
}

func (b *httpBackend) AddPickle(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	payload := map[string]any{
		"data": data,
	}
	applyDataOptions(payload, opts)
	return b.postCIDRequest(ctx, "add_pickle", payload, "add_pickle")
}

func (b *httpBackend) CalculateJSONCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	payload := map[string]any{
		"data":  data,
		"nonce": nonce,
	}
	applyDataOptions(payload, opts)
	return b.postCIDRequest(ctx, "calculate_json_cid", payload, "calculate_json_cid")
}

func (b *httpBackend) CalculatePickleCID(ctx context.Context, data any, nonce int, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	payload := map[string]any{
		"data":  data,
		"nonce": nonce,
	}
	applyDataOptions(payload, opts)
	return b.postCIDRequest(ctx, "calculate_pickle_cid", payload, "calculate_pickle_cid")
}

func (b *httpBackend) GetFileBase64(ctx context.Context, cid string, secret string) (fileData []byte, fileName string, err error) {
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

func (b *httpBackend) GetFile(ctx context.Context, cid string, secret string) (location *FileLocation, err error) {
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

func (b *httpBackend) AddYAML(ctx context.Context, data any, opts *DataOptions) (cid string, err error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("r1fs: http backend not configured")
	}
	body := map[string]any{
		"data": data,
	}
	applyDataOptions(body, opts)
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

func (b *httpBackend) GetYAML(ctx context.Context, cid string, secret string) (payload []byte, err error) {
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

func (b *httpBackend) postCIDRequest(ctx context.Context, path string, payload map[string]any, op string) (string, error) {
	jsonBody, err := encodeJSON(payload)
	if err != nil {
		return "", err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   path,
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
		return "", fmt.Errorf("r1fs: decode %s response: %w", op, err)
	}
	if strings.TrimSpace(response.CID) == "" {
		return "", fmt.Errorf("r1fs: missing cid in %s response", op)
	}
	return response.CID, nil
}

func applyDataOptions(payload map[string]any, opts *DataOptions) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.Secret) != "" {
		payload["secret"] = opts.Secret
	}
	if strings.TrimSpace(opts.Filename) != "" {
		payload["fn"] = opts.Filename
	}
	if strings.TrimSpace(opts.FilePath) != "" {
		payload["file_path"] = opts.FilePath
	}
	if opts.Nonce != nil {
		payload["nonce"] = *opts.Nonce
	}
}

func applyPathOptions(payload map[string]any, opts *DataOptions) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.Filename) != "" {
		payload["filename"] = opts.Filename
	}
	if strings.TrimSpace(opts.FilePath) != "" {
		payload["file_path"] = opts.FilePath
		if _, ok := payload["filename"]; !ok {
			payload["filename"] = opts.FilePath
		}
	}
	if strings.TrimSpace(opts.Secret) != "" {
		payload["secret"] = opts.Secret
	}
	if opts.Nonce != nil {
		payload["nonce"] = *opts.Nonce
	}
}

func resolveUploadName(opts *DataOptions) string {
	if opts == nil {
		return ""
	}
	if name := strings.TrimSpace(opts.Filename); name != "" {
		return name
	}
	if path := strings.TrimSpace(opts.FilePath); path != "" {
		return filepath.Base(path)
	}
	return ""
}

func applyBodyJSON(opts *DataOptions) map[string]any {
	meta := make(map[string]any)
	if opts == nil {
		return meta
	}
	if strings.TrimSpace(opts.Secret) != "" {
		meta["secret"] = opts.Secret
	}
	if opts.Nonce != nil {
		meta["nonce"] = *opts.Nonce
	}
	if strings.TrimSpace(opts.Filename) != "" {
		meta["fn"] = opts.Filename
	}
	if strings.TrimSpace(opts.FilePath) != "" {
		meta["file_path"] = opts.FilePath
	}
	return meta
}
