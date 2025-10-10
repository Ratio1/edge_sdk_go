package mock_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs/mock"
)

func TestMockAddFileBase64AndGetFileBase64(t *testing.T) {
	now := time.Now().UTC()
	m := mock.New(mock.WithClock(func() time.Time { return now }))
	ctx := context.Background()

	data := []byte("mock-file")
	stat, err := m.AddFileBase64(ctx, "files/a.txt", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("AddFileBase64: %v", err)
	}
	if stat.Path == "" || stat.ContentType != "text/plain" || stat.Size != int64(len(data)) {
		t.Fatalf("unexpected add_file_base64 stat: %#v", stat)
	}

	payload, err := m.GetFileBase64(ctx, stat.Path, "")
	if err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
	if !bytes.Equal(payload, data) {
		t.Fatalf("get mismatch: %q", payload)
	}

	if stat.ETag == "" || stat.LastModified == nil {
		t.Fatalf("expected ETag and LastModified: %#v", stat)
	}

	loc, err := m.GetFile(ctx, stat.Path, "")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if loc.Filename != "files/a.txt" {
		t.Fatalf("unexpected filename: %#v", loc)
	}
}

func TestMockSeed(t *testing.T) {
	m := mock.New()
	seed := []devseed.R1FSSeedEntry{
		{
			Path:        "/seed/one.txt",
			Base64:      base64.StdEncoding.EncodeToString([]byte("hello")),
			ContentType: "text/plain",
		},
	}
	if err := m.Seed(seed); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	payload, err := m.GetFileBase64(context.Background(), "/seed/one.txt", "")
	if err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
	if string(payload) != "hello" {
		t.Fatalf("unexpected payload: %q", payload)
	}
}

func TestMockAddFileAndYAML(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	fileData := []byte("stream data")
	stat, err := m.AddFile(ctx, "stream.txt", fileData, int64(len(fileData)), nil)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if stat.Path == "" {
		t.Fatalf("AddFile returned empty path")
	}

	loc, err := m.GetFile(ctx, stat.Path, "")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if loc.Filename != "stream.txt" || loc.Path == "" {
		t.Fatalf("unexpected file location: %#v", loc)
	}

	yamlCID, err := m.AddYAML(ctx, map[string]string{"hello": "world"}, "config.yaml", "")
	if err != nil {
		t.Fatalf("AddYAML: %v", err)
	}
	payload, err := m.GetYAML(ctx, yamlCID, "")
	if err != nil {
		t.Fatalf("GetYAML: %v", err)
	}
	var doc struct {
		FileData map[string]string `json:"file_data"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("decode yaml response: %v", err)
	}
	if doc.FileData["hello"] != "world" {
		t.Fatalf("unexpected yaml data: %#v", doc)
	}

	missing, err := m.GetYAML(ctx, "missing", "")
	if err != nil {
		t.Fatalf("GetYAML missing: %v", err)
	}
	if string(missing) != "\"error\"" {
		t.Fatalf("expected error response, got %s", string(missing))
	}
}
