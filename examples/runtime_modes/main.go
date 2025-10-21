package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ratio1/edge_sdk_go/pkg/cstore"
	"github.com/Ratio1/edge_sdk_go/pkg/r1fs"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cs, err := cstore.NewFromEnv()
	if err != nil {
		log.Fatalf("initialise CStore client from env: %v", err)
	}
	fs, err := r1fs.NewFromEnv()
	if err != nil {
		log.Fatalf("initialise R1FS client from env: %v", err)
	}

	fmt.Println("CStore and R1FS clients initialised from environment variables.")

	status, err := cs.GetStatus(ctx)
	if err != nil {
		log.Fatalf("fetch CStore status: %v", err)
	}
	keyCount := 0
	if status != nil {
		keyCount = len(status.Keys)
	}
	fmt.Printf("CStore /get_status reported %d keys.\n", keyCount)

	cid, err := fs.CalculateJSONCID(ctx, map[string]any{"integration": "ratio1-sdk-go"}, 3, nil)
	if err != nil {
		log.Fatalf("calculate JSON CID: %v", err)
	}
	fmt.Printf("Sample JSON CID (not persisted): %s\n", cid)
}
