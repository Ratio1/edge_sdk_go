package cstore

import (
	"errors"
	"time"
)

// Item represents a stored key/value pair.
type Item[T any] struct {
	Key       string
	Value     T
	ETag      string
	ExpiresAt *time.Time
}

// Status describes the payload returned by /get_status.
type Status struct {
	Keys []string `json:"keys"`
}

// HashItem represents a field stored under a hash key.
type HashItem[T any] struct {
	HashKey   string
	Field     string
	Value     T
	ETag      string
	ExpiresAt *time.Time
}

// SetOptions controls write semantics for Set operations.
type SetOptions struct {
	TTLSeconds  *int
	IfETagMatch string
	IfAbsent    bool
}

var (
	// ErrNotFound is returned when a key is missing.
	ErrNotFound = errors.New("cstore: not found")
	// ErrPreconditionFailed signals an optimistic concurrency failure.
	ErrPreconditionFailed = errors.New("cstore: precondition failed")
	// ErrUnsupportedFeature indicates the upstream API does not yet expose the desired capability.
	ErrUnsupportedFeature = errors.New("cstore: unsupported feature (TODO: confirm once API exposes headers)")
)
