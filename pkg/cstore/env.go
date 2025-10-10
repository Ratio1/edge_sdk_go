package cstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
)

const (
	envMode           = "R1_RUNTIME_MODE"
	envCStoreURL      = "EE_CHAINSTORE_API_URL"
	envMockCStoreSeed = "R1_MOCK_CSTORE_SEED"

	modeAuto = "auto"
	modeHTTP = "http"
	modeMock = "mock"
)

// NewFromEnv initialises a Client based on Ratio1 environment variables and
// returns the resolved mode ("http" or "mock").
func NewFromEnv() (*Client, string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(envMode)))
	baseURL := strings.TrimSpace(os.Getenv(envCStoreURL))

	switch mode {
	case "", modeAuto:
		if baseURL != "" {
			return newHTTPClient(baseURL)
		}
		return newMockClient()
	case modeHTTP:
		if baseURL == "" {
			return nil, "", fmt.Errorf("cstore: HTTP mode requires %s", envCStoreURL)
		}
		return newHTTPClient(baseURL)
	case modeMock:
		return newMockClient()
	default:
		return nil, "", fmt.Errorf("cstore: unsupported %s value %q", envMode, mode)
	}
}

func newHTTPClient(baseURL string) (*Client, string, error) {
	client, err := New(baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("cstore: init HTTP client: %w", err)
	}
	return client, modeHTTP, nil
}

func newMockClient() (*Client, string, error) {
	store := newMockStore()
	if path := strings.TrimSpace(os.Getenv(envMockCStoreSeed)); path != "" {
		entries, err := devseed.LoadCStoreSeed(path)
		if err != nil {
			return nil, "", fmt.Errorf("cstore: load mock seed: %w", err)
		}
		if err := store.seed(entries); err != nil {
			return nil, "", fmt.Errorf("cstore: apply mock seed: %w", err)
		}
	}
	return NewWithBackend(&mockBackend{store: store}), modeMock, nil
}

type mockBackend struct {
	store *mockStore
}

func (b *mockBackend) GetRaw(ctx context.Context, key string) ([]byte, error) {
	return b.store.getRaw(ctx, key)
}

func (b *mockBackend) PutRaw(ctx context.Context, key string, raw []byte, opts *PutOptions) (*Item[json.RawMessage], error) {
	ent, err := b.store.putRaw(ctx, key, raw, opts)
	if err != nil {
		return nil, err
	}
	var expires *time.Time
	if !ent.expiresAt.IsZero() {
		ex := ent.expiresAt
		expires = &ex
	}
	return &Item[json.RawMessage]{
		Key:       key,
		Value:     append([]byte(nil), ent.data...),
		ETag:      ent.etag,
		ExpiresAt: expires,
	}, nil
}

func (b *mockBackend) ListKeys(ctx context.Context) ([]string, error) {
	return b.store.listKeys(ctx)
}

func (b *mockBackend) HGetRaw(ctx context.Context, hashKey, field string) ([]byte, error) {
	return b.store.hGetRaw(ctx, hashKey, field)
}

func (b *mockBackend) HSetRaw(ctx context.Context, hashKey, field string, raw []byte, opts *PutOptions) (*HashItem[json.RawMessage], error) {
	ent, err := b.store.hSetRaw(ctx, hashKey, field, raw, opts)
	if err != nil {
		return nil, err
	}
	return &HashItem[json.RawMessage]{
		HashKey: hashKey,
		Field:   field,
		Value:   append([]byte(nil), ent.data...),
		ETag:    ent.etag,
	}, nil
}

func (b *mockBackend) HGetAllRaw(ctx context.Context, hashKey string) ([]byte, error) {
	fields, err := b.store.hGetAllRaw(ctx, hashKey)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return []byte("null"), nil
	}
	data, err := encodeHashMap(fields)
	if err != nil {
		return nil, err
	}
	return data, nil
}
