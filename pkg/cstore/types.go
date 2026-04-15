package cstore

import "errors"

// Item represents a stored key/value pair.
type Item[T any] struct {
	Key   string
	Value T
}

// Status describes the payload returned by /get_status.
type Status struct {
	Keys []string `json:"keys"`
}

// HashItem represents a field stored under a hash key.
type HashItem[T any] struct {
	HashKey string
	Field   string
	Value   T
}

// SetOptions is reserved for future write controls.
type SetOptions struct{}

// HSyncOptions controls optional parameters for hash sync requests.
type HSyncOptions struct {
	ChainstorePeers []string
}

// HSyncResult describes the payload returned by /hsync.
type HSyncResult struct {
	HashKey      string `json:"hkey"`
	SourcePeer   string `json:"source_peer"`
	MergedFields int    `json:"merged_fields"`
}

var (
	// ErrNotFound is returned when a key is missing.
	ErrNotFound = errors.New("cstore: not found")
)
