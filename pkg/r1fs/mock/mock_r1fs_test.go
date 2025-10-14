package mock_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs/mock"
)

func TestMockAddFileBase64AndGetFileBase64(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	data := []byte("mock-file")
	cid, err := m.AddFileBase64(ctx, "files/a.txt", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("AddFileBase64: %v", err)
	}
	if cid == "" {
		t.Fatalf("AddFileBase64 returned empty cid")
	}

	payload, filename, err := m.GetFileBase64(ctx, cid, "")
	if err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
	if filename == "" {
		t.Fatalf("expected filename, got empty")
	}
	if !bytes.Equal(payload, data) {
		t.Fatalf("get mismatch: %q", payload)
	}

	loc, err := m.GetFile(ctx, cid, "")
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

	payload, filename, err := m.GetFileBase64(context.Background(), "/seed/one.txt", "")
	if err != nil {
		t.Fatalf("GetFileBase64: %v", err)
	}
	if string(payload) != "hello" {
		t.Fatalf("unexpected payload: %q", payload)
	}
	if filename == "" {
		t.Fatalf("expected filename for seeded file")
	}
}

func TestMockAddFileAndYAML(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	fileData := []byte("stream data")
	cid, err := m.AddFile(ctx, "stream.txt", fileData, int64(len(fileData)), nil)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if cid == "" {
		t.Fatalf("AddFile returned empty path")
	}

	loc, err := m.GetFile(ctx, cid, "")
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
