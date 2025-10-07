# Ratio1 SDK for Go

Go clients for Ratio1 edge-node services:

- **CStore** – key/value storage with optimistic concurrency semantics.
- **R1FS** – file/object storage backed by the upstream IPFS manager.

The library mirrors the FastAPI plugins shipped in the Ratio1 edge node:

- [`cstore_manager_api.py`](https://github.com/Ratio1/edge_node/blob/main/extensions/business/cstore/cstore_manager_api.py)
- [`r1fs_manager_api.py`](https://github.com/Ratio1/edge_node/blob/main/extensions/business/r1fs/r1fs_manager_api.py)

When the official APIs lack features (TTL headers, full directory listings, deletes), the SDK documents the gap with TODO markers and returns `ErrUnsupportedFeature` where appropriate.

## Install

```bash
go get github.com/Ratio1/ratio1_sdk_go
```

## Environment variables

| Variable | Meaning |
| --- | --- |
| `CSTORE_API_URL` | Base URL for the CStore REST manager. |
| `R1FS_API_URL` | Base URL for the R1FS REST manager. |
| `R1_RUNTIME_MODE` | `auto` (default), `http`, or `mock`. `auto` picks `http` when both URLs are set, otherwise `mock`. |
| `R1_MOCK_CSTORE_SEED` | Optional JSON file with initial key/value pairs for mock mode. |
| `R1_MOCK_R1FS_SEED` | Optional JSON file with initial file definitions for mock mode. |

## Quick start

### HTTP mode

```bash
export R1_RUNTIME_MODE=http
export CSTORE_API_URL=https://example-node/cstore
export R1FS_API_URL=https://example-node/r1fs

go run ./examples/basic_http
```

### Mock mode

```bash
unset CSTORE_API_URL R1FS_API_URL
export R1_RUNTIME_MODE=mock

go run ./examples/basic_mock
```

### Sandbox server

The sandbox wraps the in-memory mocks behind HTTP endpoints so you can exercise the SDK end-to-end.

```bash
go run ./cmd/ratio1-sandbox --addr :8787

# in another terminal
export R1_RUNTIME_MODE=http
export CSTORE_API_URL=http://localhost:8787
export R1FS_API_URL=http://localhost:8787

go run ./examples/basic_http
```

Flags:

- `--kv-seed path.json`
- `--fs-seed path.json`
- `--latency 200ms`
- `--fail rate=0.05,code=500`

## Usage snippets

```go
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
```

## Development

```bash
make tidy       # go mod tidy
make build      # go build ./...
make test       # go test ./...
make sandbox    # go run ./cmd/ratio1-sandbox
make tag VERSION=v0.1.0
```

## Limitations & TODOs

- Upstream APIs currently lack TTL, delete, and list support. The SDK surfaces these gaps via `ErrUnsupportedFeature` and TODO comments pointing back to the Python sources.
- R1FS streaming is implemented via base64 payloads; a TODO tracks upgrading to streaming uploads when supported.
- CStore `Put` ignores TTL/conditional headers until the REST manager accepts them.

Bug reports and contributions are welcome through pull requests or issues in the Ratio1 organisation.
