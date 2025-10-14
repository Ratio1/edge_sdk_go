package r1fs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
)

const (
	envR1Mode       = "R1_RUNTIME_MODE"
	envR1FSURL      = "EE_R1FS_API_URL"
	envMockR1FSSeed = "R1_MOCK_R1FS_SEED"
	runtimeModeAuto = "auto"
	runtimeModeHTTP = "http"
	runtimeModeMock = "mock"
)

// NewFromEnv initialises an R1FS client based on Ratio1 environment variables
// and returns the resolved mode ("http" or "mock").
func NewFromEnv() (client *Client, mode string, err error) {
	mode = strings.ToLower(strings.TrimSpace(os.Getenv(envR1Mode)))
	baseURL := strings.TrimSpace(os.Getenv(envR1FSURL))

	switch mode {
	case "", runtimeModeAuto:
		if baseURL != "" {
			return newHTTPClient(baseURL)
		}
		return newMockClient()
	case runtimeModeHTTP:
		if baseURL == "" {
			return nil, "", fmt.Errorf("r1fs: HTTP mode requires %s", envR1FSURL)
		}
		return newHTTPClient(baseURL)
	case runtimeModeMock:
		return newMockClient()
	default:
		return nil, "", fmt.Errorf("r1fs: unsupported %s value %q", envR1Mode, mode)
	}
}

func newHTTPClient(baseURL string) (*Client, string, error) {
	client, err := New(baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("r1fs: init HTTP client: %w", err)
	}
	return client, runtimeModeHTTP, nil
}

func newMockClient() (*Client, string, error) {
	store := newMockFS()
	if path := strings.TrimSpace(os.Getenv(envMockR1FSSeed)); path != "" {
		entries, err := devseed.LoadR1FSSeed(path)
		if err != nil {
			return nil, "", fmt.Errorf("r1fs: load mock seed: %w", err)
		}
		if err := store.seed(entries); err != nil {
			return nil, "", fmt.Errorf("r1fs: apply mock seed: %w", err)
		}
	}
	return NewWithBackend(&mockBackend{store: store}), runtimeModeMock, nil
}

type mockBackend struct {
	store *mockFS
}

func (b *mockBackend) AddFileBase64(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (string, error) {
	return b.store.addFileBase64(ctx, filename, data, size, opts)
}

func (b *mockBackend) GetFileBase64(ctx context.Context, cid string, secret string) ([]byte, string, error) {
	return b.store.getFileBase64(ctx, cid, secret)
}

func (b *mockBackend) AddFile(ctx context.Context, filename string, data []byte, size int64, opts *UploadOptions) (string, error) {
	return b.store.addFile(ctx, filename, data, size, opts)
}

func (b *mockBackend) GetFile(ctx context.Context, cid string, secret string) (*FileLocation, error) {
	return b.store.getFile(ctx, cid)
}

func (b *mockBackend) AddYAML(ctx context.Context, data any, filename string, secret string) (string, error) {
	return b.store.addYAML(ctx, data, filename, secret)
}

func (b *mockBackend) GetYAML(ctx context.Context, cid string, secret string) ([]byte, error) {
	return b.store.getYAML(ctx, cid)
}
