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

// FileLocation describes the on-disk location reported by /get_file.
type FileLocation struct {
	Path     string
	Filename string
	Meta     map[string]any
}

// YAMLOptions controls additional parameters for YAML uploads.
type YAMLOptions struct {
	Filename string
	Secret   string
}

// YAMLDocument captures YAML content decoded into the requested type.
type YAMLDocument[T any] struct {
	CID  string
	Data T
}

var (
	// ErrNotFound indicates the requested file is missing.
	ErrNotFound = errors.New("r1fs: not found")
)
