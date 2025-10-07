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

type Counter struct {
	Count int `json:"count"`
}

func main() {
	ctx := context.Background()
	cs, fs, mode, err := ratio1_sdk.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap clients: %v", err)
	}
	fmt.Println("mode:", mode)

	counter := Counter{Count: 1}
	_, err = cstore.Put(ctx, cs, "jobs:123", counter, &cstore.PutOptions{})
	if err != nil {
		log.Fatalf("cstore put: %v", err)
	}
	fmt.Println("saved counter")

	item, err := cstore.Get[Counter](ctx, cs, "jobs:123")
	if err != nil {
		log.Fatalf("cstore get: %v", err)
	}

	fmt.Println("retrieved counter from cstore:", item.Value.Count)

	buf := new(bytes.Buffer)
	buf.WriteString(`{"ok":true}`)
	payload := []byte(`{"ok":true}`)
	stat, err := fs.Upload(ctx, "/outputs/result.json", bytes.NewReader(payload), int64(len(payload)), &r1fs.UploadOptions{ContentType: "application/json"})
	if err != nil {
		log.Fatalf("r1fs upload: %v", err)
	}
	fmt.Printf("uploaded %s (%d bytes)\n", stat.Path, stat.Size)

	var out bytes.Buffer
	if _, err := fs.Download(ctx, stat.Path, &out); err != nil {
		log.Fatalf("r1fs download: %v", err)
	}
	fmt.Printf("downloaded: %q\n", out.String())
}
