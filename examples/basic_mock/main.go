package main

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
	"github.com/Ratio1/ratio1_sdk_go/pkg/ratio1_sdk"
)

type Note struct {
	Text string `json:"text"`
}

func main() {
	ctx := context.Background()
	cs, fs, mode, err := ratio1_sdk.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap clients: %v", err)
	}
	fmt.Println("mode:", mode)

	if _, err := cstore.Put(ctx, cs, "notes:1", Note{Text: "hello"}, nil); err != nil {
		log.Fatalf("cstore put: %v", err)
	}
	item, err := cstore.Get[Note](ctx, cs, "notes:1")
	if err != nil {
		log.Fatalf("cstore get: %v", err)
	}
	fmt.Printf("loaded note: %+v\n", item.Value)

	payload := []byte("mock contents")
	if _, err := fs.Upload(ctx, "/files/mock.txt", bytes.NewReader(payload), int64(len(payload)), &r1fs.UploadOptions{ContentType: "text/plain"}); err != nil {
		log.Fatalf("r1fs upload: %v", err)
	}

	var out bytes.Buffer
	if _, err := fs.Download(ctx, "/files/mock.txt", &out); err != nil {
		log.Fatalf("r1fs download: %v", err)
	}
	fmt.Printf("downloaded: %q\n", out.String())
}
