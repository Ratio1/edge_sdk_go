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

// HashItem represents a field stored under a hash key.
type HashItem[T any] struct {
	HashKey   string
	Field     string
	Value     T
	ETag      string
	ExpiresAt *time.Time
}

// PutOptions controls write semantics for Put operations.
type PutOptions struct {
	TTLSeconds  *int
	IfETagMatch string
	IfAbsent    bool
}

// ListResult captures a paginated set of items.
type ListResult[T any] struct {
	Items      []Item[T]
	NextCursor string
}

var (
	// ErrNotFound is returned when a key is missing.
	ErrNotFound = errors.New("cstore: not found")
	// ErrPreconditionFailed signals an optimistic concurrency failure.
	ErrPreconditionFailed = errors.New("cstore: precondition failed")
	// ErrUnsupportedFeature indicates the upstream API does not yet expose the desired capability.
	ErrUnsupportedFeature = errors.New("cstore: unsupported feature (TODO: confirm once API exposes headers)")
)
