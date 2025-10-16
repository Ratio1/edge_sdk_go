# Ratio1 SDK for Go

Go clients for Ratio1 edge-node services:

- **CStore** – key/value storage with optimistic concurrency semantics.
- **R1FS** – file/object storage backed by the upstream IPFS manager.

The library mirrors the FastAPI plugins shipped in the Ratio1 edge node:

- [`cstore_manager_api.py`](https://github.com/Ratio1/edge_node/blob/main/extensions/business/cstore/cstore_manager_api.py)
- [`r1fs_manager_api.py`](https://github.com/Ratio1/edge_node/blob/main/extensions/business/r1fs/r1fs_manager_api.py)

When the official APIs lack features (TTL headers, directory listings, deletes), the SDK documents the gap with TODO markers and limits its surface to what the upstream currently exposes.

## Install

```bash
go get github.com/Ratio1/ratio1_sdk_go
```

or for new features in development

```bash
go get github.com/Ratio1/ratio1_sdk_go@develop
```

## Environment variables

| Variable | Meaning |
| --- | --- |
| `EE_CHAINSTORE_API_URL` | Base URL for the live CStore REST manager exposed by Ratio1 nodes. |
| `EE_R1FS_API_URL` | Base URL for the live R1FS REST manager exposed by Ratio1 nodes. |

## Quick start

The helpers `cstore.NewFromEnv` and `r1fs.NewFromEnv` read the standard Ratio1
environment variables and return ready-to-use HTTP clients. Both helpers expect
the environment variables to be populated; no in-memory or sandbox fallbacks
remain.

The examples folder contains runnable programs against live endpoints:

```bash
go run ./examples/runtime_modes   # validates environment variables and performs lightweight calls
go run ./examples/cstore          # walks through CStore write/read flows
go run ./examples/r1fs            # uploads and inspects files through R1FS
```

For local development, the [Ratio1 plugin sandbox](https://github.com/Ratio1/r1-plugins-sandbox) can emulate the CStore and R1FS APIs without hitting production endpoints.

## Usage snippets

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

type Counter struct {
	Count int `json:"count"`
}

func main() {
	ctx := context.Background()
	cs, err := cstore.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap cstore: %v", err)
	}
	fs, err := r1fs.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap r1fs: %v", err)
	}
	fmt.Println("clients ready")

	// Key/value primitives
	counter := Counter{Count: 1}
	if err := cs.Set(ctx, "jobs:123", counter, nil); err != nil {
		log.Fatalf("cstore set: %v", err)
	}
	var stored Counter
	if _, err := cs.Get(ctx, "jobs:123", &stored); err != nil {
		log.Fatalf("cstore get: %v", err)
	}
	fmt.Println("retrieved counter:", stored.Count)

	status, err := cs.GetStatus(ctx)
	if err != nil {
		log.Fatalf("cstore get_status: %v", err)
	}
	if status != nil {
		fmt.Println("stored keys:", status.Keys)
	}

	if err := cs.HSet(ctx, "jobs", "123", map[string]string{"status": "queued"}, nil); err != nil {
		log.Fatalf("cstore hset: %v", err)
	}
	hItems, err := cs.HGetAll(ctx, "jobs")
	if err != nil {
		log.Fatalf("cstore hgetall: %v", err)
	}
	for _, item := range hItems {
		var value map[string]string
		if err := json.Unmarshal(item.Value, &value); err != nil {
			log.Fatalf("decode hash field %s: %v", item.Field, err)
		}
		fmt.Printf("hash %s -> %v\n", item.Field, value)
	}

	// File primitives
	data := []byte(`{"ok":true}`)
	base64CID, err := fs.AddFileBase64(ctx, bytes.NewReader(data), &r1fs.DataOptions{FilePath: "/outputs/result.json"})
	if err != nil {
		log.Fatalf("r1fs upload: %v", err)
	}
	fmt.Printf("uploaded CID: %s\n", base64CID)

	fileCID, err := fs.AddFile(ctx, bytes.NewReader([]byte{0xde, 0xad}), &r1fs.DataOptions{Filename: "artifact.bin"})
	if err != nil {
		log.Fatalf("r1fs add_file: %v", err)
	}
	loc, err := fs.GetFile(ctx, fileCID, "")
	if err != nil {
		log.Fatalf("r1fs get_file: %v", err)
	}
	fmt.Printf("file stored at: %s (filename=%s)\n", loc.Path, loc.Filename)

	cid, err := fs.AddYAML(ctx, map[string]any{"service": "r1fs", "enabled": true}, &r1fs.DataOptions{Filename: "config.yaml"})
	if err != nil {
		log.Fatalf("r1fs add_yaml: %v", err)
	}
	var yamlDoc map[string]any
	if _, err := fs.GetYAML(ctx, cid, "", &yamlDoc); err != nil {
		log.Fatalf("r1fs get_yaml: %v", err)
	}
	fmt.Println("yaml document:", yamlDoc)

	calcCID, err := fs.CalculateJSONCID(ctx, map[string]any{"service": "r1fs"}, 42, nil)
	if err != nil {
		log.Fatalf("r1fs calculate_json_cid: %v", err)
	}
	fmt.Println("calculated cid:", calcCID)
}
```

> Prefer the per-package helpers `cstore.NewFromEnv` and `r1fs.NewFromEnv` to bootstrap clients. These ensure each service can be initialised and tested independently.

## Examples

- `examples/runtime_modes` – validates environment variables and issues lightweight calls to live endpoints.
- `examples/cstore` – runs write/read/hash operations against the configured CStore manager.
- `examples/r1fs` – uploads files and YAML documents to the configured R1FS manager.

## Development

```bash
make tidy       # go mod tidy
make build      # go build ./...
make test       # go test ./...
make tag VERSION=v0.1.0
```


Bug reports and contributions are welcome through pull requests or issues in the Ratio1 organisation.
