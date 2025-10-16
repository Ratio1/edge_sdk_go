package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

func main() {
	prefix := flag.String("prefix", "ratio1-sdk-demo", "directory prefix used for uploaded assets")
	flag.Parse()

	client, err := r1fs.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap R1FS client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	basePath := strings.Trim(strings.ReplaceAll(*prefix, "\\", "/"), "/")
	if basePath == "" {
		basePath = "ratio1-sdk-demo"
	}
	timestamp := time.Now().UnixNano()

	filePath := fmt.Sprintf("%s/%d-base64.txt", basePath, timestamp)
	payload := []byte(fmt.Sprintf("R1FS upload from Go SDK at %s", time.Now().Format(time.RFC3339)))
	fmt.Printf("Uploading %q via /add_file_base64...\n", filePath)
	cid, err := client.AddFileBase64(ctx, bytes.NewReader(payload), &r1fs.DataOptions{FilePath: filePath})
	if err != nil {
		log.Fatalf("AddFileBase64: %v", err)
	}
	fmt.Printf("CID: %s\n", cid)

	fmt.Println("Downloading the same file via /get_file_base64...")
	data, filename, err := client.GetFileBase64(ctx, cid, "")
	if err != nil {
		log.Fatalf("GetFileBase64: %v", err)
	}
	fmt.Printf("Filename: %s Size: %d Preview: %q\n", filename, len(data), sampleBytes(data, 40))

	binaryPath := fmt.Sprintf("%s/%d-binary.bin", basePath, timestamp)
	fmt.Printf("\nUploading %q via /add_file (multipart)...\n", binaryPath)
	fileCID, err := client.AddFile(ctx, bytes.NewReader([]byte{0xca, 0xfe, 0xba, 0xbe}), &r1fs.DataOptions{FilePath: binaryPath})
	if err != nil {
		log.Fatalf("AddFile: %v", err)
	}
	fmt.Printf("CID: %s\n", fileCID)

	fmt.Println("Resolving the stored file metadata via /get_file...")
	loc, err := client.GetFile(ctx, fileCID, "")
	if err != nil {
		log.Fatalf("GetFile: %v", err)
	}
	fmt.Printf("Download path: %s Filename: %s Meta: %v\n", loc.Path, loc.Filename, loc.Meta)

	fmt.Println("\nStoring YAML content via /add_yaml...")
	yamlCID, err := client.AddYAML(ctx, map[string]any{
		"service": "r1fs",
		"source":  "go-sdk",
		"time":    time.Now().Format(time.RFC3339),
	}, &r1fs.DataOptions{Filename: fmt.Sprintf("%s/%d-config.yaml", basePath, timestamp)})
	if err != nil {
		log.Fatalf("AddYAML: %v", err)
	}
	fmt.Printf("CID: %s\n", yamlCID)

	fmt.Println("Fetching YAML content via /get_yaml...")
	var yamlPayload map[string]any
	doc, err := client.GetYAML(ctx, yamlCID, "", &yamlPayload)
	if err != nil {
		log.Fatalf("GetYAML: %v", err)
	}
	fmt.Printf("CID: %s Data: %v\n", doc.CID, yamlPayload)

	fmt.Println("\nCalculating JSON and pickle CIDs without storing payloads...")
	jsonCID, err := client.CalculateJSONCID(ctx, map[string]any{"service": "r1fs", "action": "calculate"}, 1, &r1fs.DataOptions{Secret: "demo"})
	if err != nil {
		log.Fatalf("CalculateJSONCID: %v", err)
	}
	fmt.Printf("Calculated JSON CID: %s\n", jsonCID)

	pickleCID, err := client.CalculatePickleCID(ctx, map[string]any{"service": "r1fs", "action": "calculate"}, 2, nil)
	if err != nil {
		log.Fatalf("CalculatePickleCID: %v", err)
	}
	fmt.Printf("Calculated pickle CID: %s\n", pickleCID)
}

func sampleBytes(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}
	var buf bytes.Buffer
	if _, err := buf.Write(data[:limit]); err != nil {
		return string(data[:limit])
	}
	if _, err := buf.WriteString("..."); err != nil {
		return buf.String()
	}
	return buf.String()
}
