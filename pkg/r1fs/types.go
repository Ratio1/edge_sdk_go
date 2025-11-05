package r1fs

import "errors"

// DataOptions capture common optional parameters supported by R1FS uploads.
type DataOptions struct {
	Filename string
	FilePath string
	Secret   string
	Nonce    *int
}

// DeleteOptions captures optional flags shared by delete endpoints.
type DeleteOptions struct {
	UnpinRemote       *bool
	RunGC             *bool // maps to run_gc (single) or run_gc_after_all (bulk)
	CleanupLocalFiles *bool
}

// DeleteFileResult reports the outcome of a delete_file request.
type DeleteFileResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	CID     string `json:"cid"`
}

// DeleteFilesResult summarises the outcome of a delete_files request.
type DeleteFilesResult struct {
	Success      []string `json:"success"`
	Failed       []string `json:"failed"`
	Total        int      `json:"total"`
	SuccessCount int      `json:"success_count"`
	FailedCount  int      `json:"failed_count"`
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
