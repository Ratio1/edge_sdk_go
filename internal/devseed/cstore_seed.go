package devseed

import (
	"encoding/json"
	"fmt"
	"os"
)

// CStoreSeedEntry describes a key/value pair to load into the mock store.
type CStoreSeedEntry struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	TTLSeconds *int            `json:"ttl_seconds,omitempty"`
}

// LoadCStoreSeed reads JSON seed data from disk. The file is expected to
// contain an array of CStoreSeedEntry objects.
func LoadCStoreSeed(path string) ([]CStoreSeedEntry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("devseed: read cstore seed: %w", err)
	}
	var entries []CStoreSeedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("devseed: decode cstore seed: %w", err)
	}
	return entries, nil
}
