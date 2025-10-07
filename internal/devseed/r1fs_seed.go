package devseed

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// R1FSSeedEntry seeds the mock filesystem.
type R1FSSeedEntry struct {
	Path         string            `json:"path"`
	Base64       string            `json:"base64"`
	ContentType  string            `json:"content_type,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	LastModified *time.Time        `json:"last_modified,omitempty"`
}

// LoadR1FSSeed reads JSON data describing seed files.
func LoadR1FSSeed(path string) ([]R1FSSeedEntry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("devseed: read r1fs seed: %w", err)
	}
	var entries []R1FSSeedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("devseed: decode r1fs seed: %w", err)
	}
	return entries, nil
}
