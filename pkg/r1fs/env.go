package r1fs

import (
	"fmt"
	"os"
	"strings"
)

const (
	envR1FSURL = "EE_R1FS_API_URL"
)

// NewFromEnv initialises a Client using the live R1FS manager URL exported by
// Ratio1 nodes. It fails when the environment variable is unset.
func NewFromEnv() (client *Client, err error) {
	baseURL := strings.TrimSpace(os.Getenv(envR1FSURL))
	if baseURL == "" {
		return nil, fmt.Errorf("r1fs: HTTP mode requires %s", envR1FSURL)
	}
	return newHTTPClient(baseURL)
}

func newHTTPClient(baseURL string) (*Client, error) {
	client, err := New(baseURL)
	if err != nil {
		return nil, fmt.Errorf("r1fs: init HTTP client: %w", err)
	}
	return client, nil
}
