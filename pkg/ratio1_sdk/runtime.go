package ratio1_sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	cstoremock "github.com/Ratio1/ratio1_sdk_go/pkg/cstore/mock"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	r1fsmock "github.com/Ratio1/ratio1_sdk_go/pkg/r1fs/mock"
)

const (
	envMode           = "R1_RUNTIME_MODE"
	envCStoreURL      = "EE_CHAINSTORE_API_URL"
	envR1FSURL        = "EE_R1FS_API_URL"
	envMockCStoreSeed = "R1_MOCK_CSTORE_SEED"
	envMockR1FSSeed   = "R1_MOCK_R1FS_SEED"
	modeAuto          = "auto"
	modeHTTP          = "http"
	modeMock          = "mock"
)

// NewFromEnv initialises CStore and R1FS clients based on environment
// variables exposed inside Ratio1 nodes. It returns the resolved mode
// ("http" or "mock").
func NewFromEnv() (*cstore.Client, *r1fs.Client, string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(envMode)))
	cstoreURL := strings.TrimSpace(os.Getenv(envCStoreURL))
	r1fsURL := strings.TrimSpace(os.Getenv(envR1FSURL))

	switch mode {
	case modeAuto:
		if cstoreURL != "" && r1fsURL != "" {
			return newHTTPClients(cstoreURL, r1fsURL)
		}
		return newMockClients()
	case modeHTTP:
		if cstoreURL == "" || r1fsURL == "" {
			return nil, nil, "", fmt.Errorf("ratio1_sdk: HTTP mode requires %s and %s", envCStoreURL, envR1FSURL)
		}
		return newHTTPClients(cstoreURL, r1fsURL)
	case modeMock:
		return newMockClients()
	default:
		return nil, nil, "", fmt.Errorf("ratio1_sdk: unsupported %s value %q", envMode, mode)
	}
}

func newHTTPClients(cstoreURL, r1fsURL string) (*cstore.Client, *r1fs.Client, string, error) {
	cs, err := cstore.New(cstoreURL)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ratio1_sdk: init cstore HTTP client: %w", err)
	}
	fs, err := r1fs.New(r1fsURL)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ratio1_sdk: init r1fs HTTP client: %w", err)
	}
	return cs, fs, modeHTTP, nil
}

func newMockClients() (*cstore.Client, *r1fs.Client, string, error) {
	csMock := cstoremock.New()
	if path := strings.TrimSpace(os.Getenv(envMockCStoreSeed)); path != "" {
		entries, err := devseed.LoadCStoreSeed(path)
		if err != nil {
			return nil, nil, "", fmt.Errorf("ratio1_sdk: load cstore seed: %w", err)
		}
		if err := csMock.Seed(entries); err != nil {
			return nil, nil, "", fmt.Errorf("ratio1_sdk: apply cstore seed: %w", err)
		}
	}

	fsMock := r1fsmock.New()
	if path := strings.TrimSpace(os.Getenv(envMockR1FSSeed)); path != "" {
		entries, err := devseed.LoadR1FSSeed(path)
		if err != nil {
			return nil, nil, "", fmt.Errorf("ratio1_sdk: load r1fs seed: %w", err)
		}
		if err := fsMock.Seed(entries); err != nil {
			return nil, nil, "", fmt.Errorf("ratio1_sdk: apply r1fs seed: %w", err)
		}
	}

	return cstore.NewWithBackend(&cstoreMockBackend{store: csMock}), r1fs.NewWithBackend(&r1fsMockBackend{store: fsMock}), modeMock, nil
}

type cstoreMockBackend struct {
	store *cstoremock.Mock
}

func (b *cstoreMockBackend) GetRaw(ctx context.Context, key string) ([]byte, error) {
	item, err := cstoremock.Get[json.RawMessage](ctx, b.store, key)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return []byte("null"), nil
	}
	return append([]byte(nil), item.Value...), nil
}

func (b *cstoreMockBackend) PutRaw(ctx context.Context, key string, raw []byte, opts *cstore.PutOptions) (*cstore.Item[json.RawMessage], error) {
	item, err := cstoremock.Put(ctx, b.store, key, json.RawMessage(raw), opts)
	if err != nil {
		return nil, err
	}
	return &cstore.Item[json.RawMessage]{
		Key:       item.Key,
		Value:     append([]byte(nil), item.Value...),
		ETag:      item.ETag,
		ExpiresAt: item.ExpiresAt,
	}, nil
}

func (b *cstoreMockBackend) ListKeys(ctx context.Context) ([]string, error) {
	res, err := cstoremock.List[json.RawMessage](ctx, b.store, "", "", 0)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(res.Items))
	for _, item := range res.Items {
		keys = append(keys, item.Key)
	}
	sort.Strings(keys)
	return keys, nil
}

type r1fsMockBackend struct {
	store *r1fsmock.Mock
}

func (b *r1fsMockBackend) Upload(ctx context.Context, path string, data []byte, size int64, opts *r1fs.UploadOptions) (*r1fs.FileStat, error) {
	return b.store.Upload(ctx, path, bytes.NewReader(data), size, opts)
}

func (b *r1fsMockBackend) Download(ctx context.Context, path string) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := b.store.Download(ctx, path, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (b *r1fsMockBackend) Stat(ctx context.Context, path string) (*r1fs.FileStat, error) {
	return b.store.Stat(ctx, path)
}

func (b *r1fsMockBackend) List(ctx context.Context, dir string, cursor string, limit int) (*r1fs.ListResult, error) {
	return b.store.List(ctx, dir, cursor, limit)
}

func (b *r1fsMockBackend) Delete(ctx context.Context, path string) error {
	return b.store.Delete(ctx, path)
}
