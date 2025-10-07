package mock_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs/mock"
)

func TestMockUploadStatDownload(t *testing.T) {
	now := time.Now().UTC()
	m := mock.New(mock.WithClock(func() time.Time { return now }))
	ctx := context.Background()

	data := []byte("mock-file")
	stat, err := m.Upload(ctx, "/files/a.txt", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if stat.Path != "/files/a.txt" || stat.ContentType != "text/plain" || stat.Size != int64(len(data)) {
		t.Fatalf("unexpected upload stat: %#v", stat)
	}

	info, err := m.Stat(ctx, "/files/a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.ETag == "" || info.LastModified == nil {
		t.Fatalf("expected ETag and LastModified: %#v", info)
	}

	var buf bytes.Buffer
	n, err := m.Download(ctx, "/files/a.txt", &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if n != int64(len(data)) || !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("download mismatch: n=%d data=%q", n, buf.Bytes())
	}
}

func TestMockListAndDelete(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	files := map[string]string{
		"/logs/a.txt":  "A",
		"/logs/b.txt":  "B",
		"/other/c.txt": "C",
	}
	for path, content := range files {
		if _, err := m.Upload(ctx, path, bytes.NewBufferString(content), int64(len(content)), nil); err != nil {
			t.Fatalf("Upload %s: %v", path, err)
		}
	}

	list, err := m.List(ctx, "/logs", "", 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Files) != 1 || list.Files[0].Path != "/logs/a.txt" {
		t.Fatalf("unexpected first page: %#v", list)
	}

	list2, err := m.List(ctx, "/logs", list.NextCursor, 10)
	if err != nil {
		t.Fatalf("List2: %v", err)
	}
	if len(list2.Files) != 1 || list2.Files[0].Path != "/logs/b.txt" || list2.NextCursor != "" {
		t.Fatalf("unexpected second page: %#v", list2)
	}

	if err := m.Delete(ctx, "/logs/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Stat(ctx, "/logs/a.txt"); err != r1fs.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
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

	stat, err := m.Stat(context.Background(), "/seed/one.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.ContentType != "text/plain" {
		t.Fatalf("unexpected content type: %#v", stat)
	}
}
