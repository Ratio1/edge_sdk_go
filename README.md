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

## Environment variables

| Variable | Meaning |
| --- | --- |
| `EE_CHAINSTORE_API_URL` | Base URL for the CStore REST manager. Already available on Ratio1 nodes. |
| `EE_R1FS_API_URL` | Base URL for the R1FS REST manager. Already available on Ratio1 nodes. | 
| `R1_RUNTIME_MODE` | `auto`, `http`, or `mock`. `auto` picks `http` when both URLs are set, otherwise `mock`. |
| `R1_MOCK_CSTORE_SEED` | Optional JSON file with initial key/value pairs for mock mode. |
| `R1_MOCK_R1FS_SEED` | Optional JSON file with initial file definitions for mock mode. |

## Quick start

The helpers `cstore.NewFromEnv` and `r1fs.NewFromEnv` read the standard Ratio1
environment variables and return ready-to-use clients. Modes behave as follows:

- `http` – connect to the live REST managers (`EE_CHAINSTORE_API_URL`, `EE_R1FS_API_URL`).
- `auto` – use HTTP when both URLs are set, otherwise fall back to mocks.
- `mock` – in-memory stores, optionally seeded via `R1_MOCK_CSTORE_SEED` and `R1_MOCK_R1FS_SEED`.

The examples folder contains runnable programs covering each scenario:

```bash
go run ./examples/runtime_modes   # demonstrates http/auto/mock initialisation
go run ./examples/cstore          # walks through every cstore API
go run ./examples/r1fs            # showcases the r1fs API surface
```

### Sandbox server

The sandbox wraps the in-memory mocks behind HTTP endpoints so you can exercise the SDK end-to-end without standing up the Python managers. Every time it starts, it prints export statements that you can paste into your shell to point the SDK at the sandbox.

#### Download the release binary

Each tagged release includes pre-built archives named `ratio1-sandbox_<os>_<arch>.<ext>` (`.tar.gz` for macOS/Linux, `.zip` for Windows). Grab the one for your platform and place the binary on your PATH:

```bash
# macOS arm64 example
curl -L https://github.com/Ratio1/ratio1_sdk_go/releases/latest/download/ratio1-sandbox_darwin_arm64.tar.gz \
  | tar -xz
chmod +x ratio1-sandbox
./ratio1-sandbox --addr :8787
```

Windows users can download `ratio1-sandbox_windows_amd64.zip`, unzip it, and run `ratio1-sandbox.exe`.

Copy the `export` lines that the server prints into your shell (or redirect them into a file and `source` it) and then run your application or any of the examples:

```bash
go run ./examples/runtime_modes
```

#### Run from source

If you prefer to rebuild locally:

```bash
go run ./cmd/ratio1-sandbox --addr :8787
```

#### Flags and behaviours

- `--kv-seed path.json` – seed initial CStore keys.
- `--fs-seed path.json` – seed initial R1FS files.
- `--latency 200ms` – inject fixed latency before every request.
- `--fail rate=0.05,code=500` – randomly inject HTTP failures.

The sandbox mounts both APIs under the same host and supports the endpoints used by the SDK (CStore: `/set`, `/get`, `/get_status`, `/hset`, `/hget`, `/hgetall`; R1FS: `/add_file_base64`, `/add_file`, `/get_file_base64`, `/get_file`, `/add_yaml`, `/get_yaml`, `/get_status_r1fs`). Point both `EE_CHAINSTORE_API_URL` and `EE_R1FS_API_URL` to the address shown in the startup banner when developing against the sandbox.

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
	cs, cMode, err := cstore.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap cstore: %v", err)
	}
	fs, fMode, err := r1fs.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap r1fs: %v", err)
	}
	fmt.Println("modes:", cMode, fMode)

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
    base64CID, err := fs.AddFileBase64(ctx, "/outputs/result.json", bytes.NewReader(data), int64(len(data)), &r1fs.UploadOptions{ContentType: "application/json"})
    if err != nil {
        log.Fatalf("r1fs upload: %v", err)
    }
    fmt.Printf("uploaded CID: %s\n", base64CID)

    fileCID, err := fs.AddFile(ctx, "artifact.bin", bytes.NewReader([]byte{0xde, 0xad}), 2, nil)
    if err != nil {
        log.Fatalf("r1fs add_file: %v", err)
    }
    loc, err := fs.GetFile(ctx, fileCID, "")
	if err != nil {
		log.Fatalf("r1fs get_file: %v", err)
	}
	fmt.Printf("file stored at: %s (filename=%s)\n", loc.Path, loc.Filename)

	cid, err := fs.AddYAML(ctx, map[string]any{"service": "r1fs", "enabled": true}, &r1fs.YAMLOptions{Filename: "config.yaml"})
	if err != nil {
		log.Fatalf("r1fs add_yaml: %v", err)
	}
	var yamlDoc map[string]any
	if _, err := fs.GetYAML(ctx, cid, "", &yamlDoc); err != nil {
		log.Fatalf("r1fs get_yaml: %v", err)
	}
	fmt.Println("yaml document:", yamlDoc)
}
```

> Prefer the per-package helpers `cstore.NewFromEnv` and `r1fs.NewFromEnv` to bootstrap clients. These ensure each service can be initialised and tested independently.

## Examples

- `examples/runtime_modes` – spins up local test servers and shows how `http`, `auto`, and `mock` resolution behaves.
- `examples/cstore` – exercises the supported CStore operations (Set/Get/HSet/HGet/HGetAll/GetStatus).
- `examples/r1fs` – demonstrates uploads, metadata lookups, and YAML helpers against a simulated manager.

## Development

```bash
make tidy       # go mod tidy
make build      # go build ./...
make test       # go test ./...
make sandbox    # go run ./cmd/ratio1-sandbox
make sandbox-dist  # build dist/ratio1-sandbox_<os>_<arch>.tar.gz for release uploads
make tag VERSION=v0.1.0
```


Bug reports and contributions are welcome through pull requests or issues in the Ratio1 organisation.
