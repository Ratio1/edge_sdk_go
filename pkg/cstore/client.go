package cstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/httpx"
	"github.com/Ratio1/ratio1_sdk_go/internal/ratio1api"
)

// Client provides access to the upstream CStore REST API.
type Client struct {
	backend Backend
}

// New constructs a Client bound to the provided base URL.
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

// NewWithBackend allows callers to supply a custom backend (e.g., mocks).
func NewWithBackend(b Backend) *Client {
	return &Client{backend: b}
}

// Get retrieves a value and decodes it into the requested type.
func Get[T any](ctx context.Context, client *Client, key string) (*Item[T], error) {
	return getItem[T](ctx, client, key)
}

// Put stores a value encoded as JSON.
func Put[T any](ctx context.Context, client *Client, key string, value T, opts *PutOptions) (*Item[T], error) {
	return putItem(ctx, client, key, value, opts)
}

// HGet retrieves a value stored under a hash key and decodes it into the requested type.
func HGet[T any](ctx context.Context, client *Client, hashKey, field string) (*HashItem[T], error) {
	return getHashItem[T](ctx, client, hashKey, field)
}

// HSet stores a field value within a hash key.
func HSet[T any](ctx context.Context, client *Client, hashKey, field string, value T, opts *PutOptions) (*HashItem[T], error) {
	return putHashItem(ctx, client, hashKey, field, value, opts)
}

// HGetAll retrieves all fields stored under a hash key.
func HGetAll[T any](ctx context.Context, client *Client, hashKey string) ([]HashItem[T], error) {
	return getAllHashItems[T](ctx, client, hashKey)
}

// List enumerates keys using the /get_status endpoint and returns decoded items.
func List[T any](ctx context.Context, client *Client, prefix string, cursor string, limit int) (*ListResult[T], error) {
	return listItems[T](ctx, client, prefix, cursor, limit)
}

// Delete is not currently supported by the upstream API.
func (c *Client) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("%w: delete endpoint missing in cstore_manager_api.py", ErrUnsupportedFeature)
}

// GetJSON fetches the raw JSON payload stored for a key.
func (c *Client) GetJSON(ctx context.Context, key string) ([]byte, error) {
	return getRaw(ctx, c, key)
}

// PutJSON stores a pre-encoded JSON payload (as string).
func (c *Client) PutJSON(ctx context.Context, key string, jsonPayload string, opts *PutOptions) (*Item[json.RawMessage], error) {
	item, err := putItem(ctx, c, key, json.RawMessage(jsonPayload), opts)
	if err != nil {
		return nil, err
	}
	return &Item[json.RawMessage]{
		Key:       item.Key,
		Value:     json.RawMessage(jsonPayload),
		ETag:      item.ETag,
		ExpiresAt: item.ExpiresAt,
	}, nil
}

func getItem[T any](ctx context.Context, client *Client, key string) (*Item[T], error) {
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	data, err := client.backend.GetRaw(ctx, key)
	if err != nil {
		return nil, err
	}
	return decodeItem[T](key, data)
}

func putItem[T any](ctx context.Context, client *Client, key string, value T, opts *PutOptions) (*Item[T], error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("cstore: key is required")
	}
	if err := validatePutOptions(opts); err != nil {
		return nil, err
	}

	payloadBytes, err := jsonMarshal(value)
	if err != nil {
		return nil, fmt.Errorf("cstore: encode value: %w", err)
	}

	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}

	meta, err := client.backend.PutRaw(ctx, key, payloadBytes, opts)
	if err != nil {
		return nil, err
	}

	item := &Item[T]{Key: key, Value: value}
	if meta != nil {
		item.ETag = meta.ETag
		item.ExpiresAt = meta.ExpiresAt
	}
	return item, nil
}

func getHashItem[T any](ctx context.Context, client *Client, hashKey, field string) (*HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("cstore: hash field is required")
	}
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	data, err := client.backend.HGetRaw(ctx, hashKey, field)
	if err != nil {
		return nil, err
	}
	return decodeHashItem[T](hashKey, field, data)
}

func putHashItem[T any](ctx context.Context, client *Client, hashKey, field string, value T, opts *PutOptions) (*HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("cstore: hash field is required")
	}
	if err := validatePutOptions(opts); err != nil {
		return nil, err
	}

	payloadBytes, err := jsonMarshal(value)
	if err != nil {
		return nil, fmt.Errorf("cstore: encode hash value: %w", err)
	}

	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}

	meta, err := client.backend.HSetRaw(ctx, hashKey, field, payloadBytes, opts)
	if err != nil {
		return nil, err
	}

	item := &HashItem[T]{HashKey: hashKey, Field: field, Value: value}
	if meta != nil {
		item.ETag = meta.ETag
		item.ExpiresAt = meta.ExpiresAt
	}
	return item, nil
}

func getAllHashItems[T any](ctx context.Context, client *Client, hashKey string) ([]HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("cstore: hash key is required")
	}
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	data, err := client.backend.HGetAllRaw(ctx, hashKey)
	if err != nil {
		return nil, err
	}
	return decodeHashItems[T](hashKey, data)
}

func listItems[T any](ctx context.Context, client *Client, prefix string, cursor string, limit int) (*ListResult[T], error) {
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}

	keys, err := client.backend.ListKeys(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			filtered = append(filtered, k)
		}
	}
	sort.Strings(filtered)

	start := 0
	if cursor != "" {
		idx := sort.SearchStrings(filtered, cursor)
		for idx < len(filtered) && filtered[idx] <= cursor {
			idx++
		}
		start = idx
	}
	if start > len(filtered) {
		start = len(filtered)
	}

	end := len(filtered)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	items := make([]Item[T], 0, end-start)
	for _, key := range filtered[start:end] {
		item, err := getItem[T](ctx, client, key)
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, *item)
		}
	}

	nextCursor := ""
	if end < len(filtered) && end > 0 {
		nextCursor = filtered[end-1]
	}

	return &ListResult[T]{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func getRaw(ctx context.Context, client *Client, key string) ([]byte, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("cstore: key is required")
	}

	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	return client.backend.GetRaw(ctx, key)
}

func decodeItem[T any](key string, data []byte) (*Item[T], error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, fmt.Errorf("cstore: decode value: %w", err)
	}
	return &Item[T]{Key: key, Value: value}, nil
}

func decodeHashItem[T any](hashKey, field string, data []byte) (*HashItem[T], error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, fmt.Errorf("cstore: decode hash value: %w", err)
	}
	return &HashItem[T]{HashKey: hashKey, Field: field, Value: value}, nil
}

func decodeHashItems[T any](hashKey string, data []byte) ([]HashItem[T], error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, fmt.Errorf("cstore: decode hash map: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}

	fields := make([]string, 0, len(raw))
	for field := range raw {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	items := make([]HashItem[T], 0, len(fields))
	for _, field := range fields {
		var value T
		if err := json.Unmarshal(raw[field], &value); err != nil {
			return nil, fmt.Errorf("cstore: decode hash field %q: %w", field, err)
		}
		items = append(items, HashItem[T]{HashKey: hashKey, Field: field, Value: value})
	}
	return items, nil
}

func validatePutOptions(opts *PutOptions) error {
	if opts == nil {
		return nil
	}
	if opts.TTLSeconds != nil {
		// TODO: support once upstream exposes TTL semantics.
		return fmt.Errorf("%w: TTLSeconds not yet supported", ErrUnsupportedFeature)
	}
	if opts.IfETagMatch != "" || opts.IfAbsent {
		// TODO: map optimistic concurrency to upstream headers when available.
		return fmt.Errorf("%w: conditional writes not yet supported", ErrUnsupportedFeature)
	}
	return nil
}

func jsonMarshal[T any](value T) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
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

type Backend interface {
	GetRaw(ctx context.Context, key string) ([]byte, error)
	PutRaw(ctx context.Context, key string, raw []byte, opts *PutOptions) (*Item[json.RawMessage], error)
	ListKeys(ctx context.Context) ([]string, error)
	HGetRaw(ctx context.Context, hashKey, field string) ([]byte, error)
	HSetRaw(ctx context.Context, hashKey, field string, raw []byte, opts *PutOptions) (*HashItem[json.RawMessage], error)
	HGetAllRaw(ctx context.Context, hashKey string) ([]byte, error)
}

type httpBackend struct {
	client *httpx.Client
}

func (b *httpBackend) GetRaw(ctx context.Context, key string) ([]byte, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "get",
		Query:  url.Values{"key": {key}},
	})
	if err != nil {
		return nil, err
	}
	data, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := ratio1api.ExtractResult(data)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload, nil
}

func (b *httpBackend) PutRaw(ctx context.Context, key string, raw []byte, opts *PutOptions) (*Item[json.RawMessage], error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	body, err := encodeJSON(map[string]any{
		"key":              key,
		"value":            string(raw),
		"chainstore_peers": []string{},
	})
	if err != nil {
		return nil, err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "set",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   bytes.NewReader(body),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
	}
	resp, err := b.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	return nil, nil
}

func (b *httpBackend) ListKeys(ctx context.Context) ([]string, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "get_status",
	})
	if err != nil {
		return nil, err
	}
	data, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Keys []string `json:"keys"`
	}
	if err := ratio1api.DecodeResult(data, &payload); err != nil {
		return nil, fmt.Errorf("cstore: decode get_status response: %w", err)
	}
	return payload.Keys, nil
}

func (b *httpBackend) HGetRaw(ctx context.Context, hashKey, field string) ([]byte, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "hget",
		Query:  url.Values{"hkey": {hashKey}, "key": {field}},
	})
	if err != nil {
		return nil, err
	}
	data, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := ratio1api.ExtractResult(data)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload, nil
}

func (b *httpBackend) HSetRaw(ctx context.Context, hashKey, field string, raw []byte, opts *PutOptions) (*HashItem[json.RawMessage], error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	body, err := encodeJSON(map[string]any{
		"hkey":             hashKey,
		"key":              field,
		"value":            string(raw),
		"chainstore_peers": []string{},
	})
	if err != nil {
		return nil, err
	}
	req := &httpx.Request{
		Method: http.MethodPost,
		Path:   "hset",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   bytes.NewReader(body),
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
	}
	resp, err := b.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	return nil, nil
}

func (b *httpBackend) HGetAllRaw(ctx context.Context, hashKey string) ([]byte, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	resp, err := b.client.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "hgetall",
		Query:  url.Values{"hkey": {hashKey}},
	})
	if err != nil {
		return nil, err
	}
	data, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	payload, err := ratio1api.ExtractResult(data)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload, nil
}
