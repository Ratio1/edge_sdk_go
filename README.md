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
| `EE_CHAINSTORE_API_URL` | Base URL for the CStore REST manager. Already available on Ratio1 nodes. |
| `EE_R1FS_API_URL` | Base URL for the R1FS REST manager. Already available on Ratio1 nodes. | 
| `R1_RUNTIME_MODE` | `auto`, `http`, or `mock`. `auto` picks `http` when both URLs are set, otherwise `mock`. |
| `R1_MOCK_CSTORE_SEED` | Optional JSON file with initial key/value pairs for mock mode. |
| `R1_MOCK_R1FS_SEED` | Optional JSON file with initial file definitions for mock mode. |

## Quick start

### HTTP mode

```bash
export R1_RUNTIME_MODE=http
export EE_CHAINSTORE_API_URL=https://example-node/cstore
export EE_R1FS_API_URL=https://example-node/r1fs

go run ./examples/basic_http
```

### Mock mode

```bash
unset EE_CHAINSTORE_API_URL EE_R1FS_API_URL
export R1_RUNTIME_MODE=mock

go run ./examples/basic_mock
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

Copy the `export` lines that the server prints into your shell (or redirect them into a file and `source` it) and then run your application or the examples:

```bash
go run ./examples/basic_http
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

The sandbox mounts both APIs under the same host and supports the endpoints used by the SDK (`/set`, `/get`, `/get_status` for CStore; `/add_file_base64`, `/get_file_base64`, `/get_status_r1fs` for R1FS). Point both `EE_CHAINSTORE_API_URL` and `EE_R1FS_API_URL` to the address shown in the startup banner when developing against the sandbox.

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

> Prefer the per-package helpers `cstore.NewFromEnv` and `r1fs.NewFromEnv` to bootstrap clients. The legacy `ratio1_sdk.NewFromEnv` wrapper is still available if you want both handles at once.

## Development

```bash
make tidy       # go mod tidy
make build      # go build ./...
make test       # go test ./...
make sandbox    # go run ./cmd/ratio1-sandbox
make sandbox-dist  # build dist/ratio1-sandbox_<os>_<arch>.tar.gz for release uploads
make tag VERSION=v0.1.0
```

## Limitations & TODOs

- Upstream APIs currently lack TTL, delete, and list support. The SDK surfaces these gaps via `ErrUnsupportedFeature` and TODO comments pointing back to the Python sources.
- R1FS streaming is implemented via base64 payloads; a TODO tracks upgrading to streaming uploads when supported.
- CStore `Put` ignores TTL/conditional headers until the REST manager accepts them.

Bug reports and contributions are welcome through pull requests or issues in the Ratio1 organisation.
