package r1fs

import (
	"fmt"
	"os"
	"strings"
)

const (
	envR1FSURL = "EE_R1FS_API_URL"
)

// NewFromEnv initialises an R1FS client based on Ratio1 environment variables
// and returns the resolved mode ("http" or "mock").
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
