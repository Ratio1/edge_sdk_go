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
	cid, err := m.AddFileBase64(ctx, bytes.NewReader(data), &r1fs.DataOptions{FilePath: "files/a.txt"})
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
	cid, err := m.AddFile(ctx, bytes.NewReader(fileData), &r1fs.DataOptions{Filename: "stream.txt"})
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

	yamlCID, err := m.AddYAML(ctx, map[string]string{"hello": "world"}, &r1fs.DataOptions{Filename: "config.yaml"})
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

func TestMockStructuredData(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	jsonNonce := 9
	jsonCID, err := m.AddJSON(ctx, map[string]string{"hello": "world"}, &r1fs.DataOptions{Filename: "data.json", Secret: "sec", Nonce: &jsonNonce})
	if err != nil {
		t.Fatalf("AddJSON: %v", err)
	}
	if jsonCID == "" {
		t.Fatalf("AddJSON returned empty cid")
	}
	loc, err := m.GetFile(ctx, jsonCID, "")
	if err != nil {
		t.Fatalf("GetFile json: %v", err)
	}
	if loc.Filename != "data.json" {
		t.Fatalf("expected overridden filename, got %#v", loc)
	}
	calcJSON, err := m.CalculateJSONCID(ctx, map[string]string{"hello": "world"}, 77, &r1fs.DataOptions{Secret: "sec"})
	if err != nil {
		t.Fatalf("CalculateJSONCID: %v", err)
	}
	if calcJSON == "" {
		t.Fatalf("CalculateJSONCID returned empty cid")
	}
	pickleCID, err := m.AddPickle(ctx, map[string]int{"value": 1}, &r1fs.DataOptions{FilePath: "test/your/path"})
	if err != nil {
		t.Fatalf("AddPickle: %v", err)
	}
	if pickleCID == "" {
		t.Fatalf("AddPickle returned empty cid")
	}
	calcPickle, err := m.CalculatePickleCID(ctx, map[string]int{"value": 1}, 33, nil)
	if err != nil {
		t.Fatalf("CalculatePickleCID: %v", err)
	}
	if calcPickle == "" {
		t.Fatalf("CalculatePickleCID returned empty cid")
	}
}
