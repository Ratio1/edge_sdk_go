package r1fs

import "errors"

// DataOptions capture common optional parameters supported by R1FS uploads.
type DataOptions struct {
	Filename string
	FilePath string
	Secret   string
	Nonce    *int
}

// FileLocation describes the on-disk location reported by /get_file.
type FileLocation struct {
	Path     string
	Filename string
	Meta     map[string]any
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
