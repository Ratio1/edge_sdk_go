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
	"strconv"
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

// Get retrieves a value as raw JSON. Provide out to decode into a struct.
func (c *Client) Get(ctx context.Context, key string, out any) (*Item[json.RawMessage], error) {
	item, err := getItem[json.RawMessage](ctx, c, key)
	if err != nil || item == nil {
		return item, err
	}
	if out != nil {
		if err := json.Unmarshal(item.Value, out); err != nil {
			return nil, fmt.Errorf("cstore: decode value: %w", err)
		}
	}
	return item, nil
}

// Set stores a value encoded as JSON.
func (c *Client) Set(ctx context.Context, key string, value any, opts *SetOptions) (*Item[json.RawMessage], error) {
	return setJSONEncoded(ctx, c, key, value, opts)
}

// HGet retrieves a value stored under a hash key and decodes it into the requested type.
func (c *Client) HGet(ctx context.Context, hashKey, field string, out any) (*HashItem[json.RawMessage], error) {
	item, err := getHashItem[json.RawMessage](ctx, c, hashKey, field)
	if err != nil || item == nil {
		return item, err
	}
	if out != nil {
		if err := json.Unmarshal(item.Value, out); err != nil {
			return nil, fmt.Errorf("cstore: decode hash value: %w", err)
		}
	}
	return item, nil
}

// HSet stores a field value within a hash key.
func (c *Client) HSet(ctx context.Context, hashKey, field string, value any, opts *SetOptions) (*HashItem[json.RawMessage], error) {
	return setHashJSONEncoded(ctx, c, hashKey, field, value, opts)
}

// HGetAll retrieves all fields stored under a hash key.
func (c *Client) HGetAll(ctx context.Context, hashKey string) ([]HashItem[json.RawMessage], error) {
	return getAllHashItems[json.RawMessage](ctx, c, hashKey)
}

// GetStatus returns the payload exposed by the /get_status endpoint.
func (c *Client) GetStatus(ctx context.Context) (*Status, error) {
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	payload, err := c.backend.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var status Status
	if err := json.Unmarshal(trimmed, &status); err != nil {
		return nil, fmt.Errorf("cstore: decode get_status payload: %w", err)
	}
	return &status, nil
}
func getItem[T any](ctx context.Context, client *Client, key string) (*Item[T], error) {
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	data, err := client.backend.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return decodeItem[T](key, data)
}

func setJSONEncoded(ctx context.Context, client *Client, key string, value any, opts *SetOptions) (*Item[json.RawMessage], error) {
	payloadBytes, err := marshalJSON(value)
	if err != nil {
		return nil, fmt.Errorf("cstore: encode value: %w", err)
	}
	return setRawJSON(ctx, client, key, payloadBytes, opts)
}

func setRawJSON(ctx context.Context, client *Client, key string, payload []byte, opts *SetOptions) (*Item[json.RawMessage], error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("cstore: key is required")
	}
	if err := validateSetOptions(opts); err != nil {
		return nil, err
	}

	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}

	raw := append([]byte(nil), bytes.TrimSpace(payload)...)

	meta, err := client.backend.Set(ctx, key, raw, opts)
	if err != nil {
		return nil, err
	}

	item := &Item[json.RawMessage]{
		Key:   key,
		Value: json.RawMessage(raw),
	}
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
	data, err := client.backend.HGet(ctx, hashKey, field)
	if err != nil {
		return nil, err
	}
	return decodeHashItem[T](hashKey, field, data)
}

func getAllHashItems[T any](ctx context.Context, client *Client, hashKey string) ([]HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("cstore: hash key is required")
	}
	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}
	data, err := client.backend.HGetAll(ctx, hashKey)
	if err != nil {
		return nil, err
	}
	return decodeHashItems[T](hashKey, data)
}

func setHashJSONEncoded(ctx context.Context, client *Client, hashKey, field string, value any, opts *SetOptions) (*HashItem[json.RawMessage], error) {
	payloadBytes, err := marshalJSON(value)
	if err != nil {
		return nil, fmt.Errorf("cstore: encode hash value: %w", err)
	}
	return setHashRawJSON(ctx, client, hashKey, field, payloadBytes, opts)
}

func setHashRawJSON(ctx context.Context, client *Client, hashKey, field string, payload []byte, opts *SetOptions) (*HashItem[json.RawMessage], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("cstore: hash field is required")
	}
	if err := validateSetOptions(opts); err != nil {
		return nil, err
	}

	if client == nil || client.backend == nil {
		return nil, fmt.Errorf("cstore: client is nil")
	}

	raw := append([]byte(nil), bytes.TrimSpace(payload)...)

	meta, err := client.backend.HSet(ctx, hashKey, field, raw, opts)
	if err != nil {
		return nil, err
	}

	item := &HashItem[json.RawMessage]{
		HashKey: hashKey,
		Field:   field,
		Value:   json.RawMessage(raw),
	}
	if meta != nil {
		item.ETag = meta.ETag
		item.ExpiresAt = meta.ExpiresAt
	}
	return item, nil
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

func validateSetOptions(opts *SetOptions) error {
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

func marshalJSON(value any) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func coerceBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return false, fmt.Errorf("empty string")
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, err
		}
		return b, nil
	case float64:
		return v != 0, nil
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return false, err
		}
		return i != 0, nil
	case nil:
		return false, fmt.Errorf("nil result")
	default:
		return false, fmt.Errorf("unexpected type %T", v)
	}
}

func decodeBoolResult(body []byte) (bool, error) {
	var raw any
	if err := ratio1api.DecodeResult(body, &raw); err != nil {
		return false, err
	}
	return coerceBool(raw)
}

type Backend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, raw []byte, opts *SetOptions) (*Item[json.RawMessage], error)
	HGet(ctx context.Context, hashKey, field string) ([]byte, error)
	HSet(ctx context.Context, hashKey, field string, raw []byte, opts *SetOptions) (*HashItem[json.RawMessage], error)
	HGetAll(ctx context.Context, hashKey string) ([]byte, error)
	GetStatus(ctx context.Context) ([]byte, error)
}

type httpBackend struct {
	client *httpx.Client
}

func (b *httpBackend) Get(ctx context.Context, key string) ([]byte, error) {
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

func (b *httpBackend) Set(ctx context.Context, key string, raw []byte, opts *SetOptions) (*Item[json.RawMessage], error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	type setRequest struct {
		Key             string          `json:"key"`
		Value           json.RawMessage `json:"value"`
		ChainstorePeers []string        `json:"chainstore_peers"`
	}
	reqPayload := setRequest{
		Key:             key,
		Value:           json.RawMessage(append([]byte(nil), raw...)),
		ChainstorePeers: []string{},
	}
	body, err := marshalJSON(reqPayload)
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

	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	ok, err := decodeBoolResult(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("cstore: decode set response: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("cstore: set rejected by upstream")
	}
	return nil, nil
}

func (b *httpBackend) GetStatus(ctx context.Context) ([]byte, error) {
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
	payload, err := ratio1api.ExtractResult(data)
	if err != nil {
		return nil, fmt.Errorf("cstore: decode get_status response: %w", err)
	}
	if payload == nil {
		return nil, nil
	}
	return payload, nil
}

func (b *httpBackend) HGet(ctx context.Context, hashKey, field string) ([]byte, error) {
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

func (b *httpBackend) HSet(ctx context.Context, hashKey, field string, raw []byte, opts *SetOptions) (*HashItem[json.RawMessage], error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("cstore: http backend not configured")
	}
	type hashSetRequest struct {
		HashKey         string          `json:"hkey"`
		Field           string          `json:"key"`
		Value           json.RawMessage `json:"value"`
		ChainstorePeers []string        `json:"chainstore_peers"`
	}
	reqPayload := hashSetRequest{
		HashKey:         hashKey,
		Field:           field,
		Value:           json.RawMessage(append([]byte(nil), raw...)),
		ChainstorePeers: []string{},
	}
	body, err := marshalJSON(reqPayload)
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
	payloadBytes, err := httpx.ReadAllAndClose(resp.Body)
	if err != nil {
		return nil, err
	}
	ok, err := decodeBoolResult(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("cstore: decode hset response: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("cstore: hset rejected by upstream")
	}
	return nil, nil
}

func (b *httpBackend) HGetAll(ctx context.Context, hashKey string) ([]byte, error) {
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
