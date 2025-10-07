package r1fs

import (
	"errors"
	"time"
)

// FileStat describes a stored file.
type FileStat struct {
	Path         string
	Size         int64
	ContentType  string
	ETag         string
	LastModified *time.Time
	Metadata     map[string]string
}

// UploadOptions control how data is written.
type UploadOptions struct {
	ContentType string
	Metadata    map[string]string
	Secret      string // TODO: align with upstream API once metadata headers are formalised.
}

// ListResult contains paginated listing results.
type ListResult struct {
	Files      []FileStat
	NextCursor string
}

var (
	// ErrNotFound indicates the requested file is missing.
	ErrNotFound = errors.New("r1fs: not found")
	// ErrUnsupportedFeature documents gaps between the desired SDK surface and upstream API support.
	ErrUnsupportedFeature = errors.New("r1fs: unsupported feature (TODO: confirm once r1fs_manager_api.py exposes it)")
)
