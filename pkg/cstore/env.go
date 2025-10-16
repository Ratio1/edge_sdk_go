package cstore

import (
	"fmt"
	"os"
	"strings"
)

const (
	envCStoreURL = "EE_CHAINSTORE_API_URL"
)

// NewFromEnv initialises a Client based on Ratio1 environment variables and
// returns the resolved mode ("http" or "mock").
func NewFromEnv() (client *Client, err error) {
	baseURL := strings.TrimSpace(os.Getenv(envCStoreURL))
	if baseURL == "" {
		return nil, fmt.Errorf("cstore: HTTP env requires %s", envCStoreURL)
	}
	return newHTTPClient(baseURL)

}

func newHTTPClient(baseURL string) (*Client, error) {
	client, err := New(baseURL)
	if err != nil {
		return nil, fmt.Errorf("cstore: init HTTP client: %w", err)
	}
	return client, nil
}
