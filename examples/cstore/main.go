package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ratio1/edge_sdk_go/pkg/cstore"
)

type counter struct {
	Count int `json:"count"`
}

func main() {
	keyPrefix := flag.String("key-prefix", "ratio1-sdk-demo", "prefix for keys written by the example")
	flag.Parse()

	client, err := cstore.NewFromEnv()
	if err != nil {
		log.Fatalf("bootstrap CStore client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prefix := strings.TrimSuffix(strings.TrimSpace(*keyPrefix), ":")
	if prefix == "" {
		prefix = "ratio1-sdk-demo"
	}
	kvKey := fmt.Sprintf("%s:%d", prefix, time.Now().UnixNano())
	hashKey := fmt.Sprintf("%s:hash", prefix)

	fmt.Printf("Writing counter to %q...\n", kvKey)
	if err := client.Set(ctx, kvKey, counter{Count: 1}, nil); err != nil {
		log.Fatalf("Set %s: %v", kvKey, err)
	}

	var stored counter
	item, err := client.Get(ctx, kvKey, &stored)
	if err != nil {
		log.Fatalf("Get %s: %v", kvKey, err)
	}
	fmt.Printf("Fetched %q -> %+v (raw payload: %s)\n", kvKey, stored, string(item.Value))

	fmt.Printf("\nHash operations on %q...\n", hashKey)
	if err := client.HSet(ctx, hashKey, "demo", map[string]any{"status": "queued"}, nil); err != nil {
		log.Fatalf("HSet %s demo: %v", hashKey, err)
	}

	var hashValue map[string]any
	hashItem, err := client.HGet(ctx, hashKey, "demo", &hashValue)
	if err != nil {
		log.Fatalf("HGet %s demo: %v", hashKey, err)
	}
	fmt.Printf("Hash field %q -> %v (raw payload: %s)\n", hashItem.Field, hashValue, string(hashItem.Value))

	all, err := client.HGetAll(ctx, hashKey)
	if err != nil {
		log.Fatalf("HGetAll %s: %v", hashKey, err)
	}
	fmt.Printf("All hash fields for %q (%d entries):\n", hashKey, len(all))
	for _, entry := range all {
		var decoded map[string]any
		if err := json.Unmarshal(entry.Value, &decoded); err != nil {
			log.Fatalf("decode hash entry %s: %v", entry.Field, err)
		}
		fmt.Printf("  %s => %v\n", entry.Field, decoded)
	}

	fmt.Println("\nFetching service status...")
	status, err := client.GetStatus(ctx)
	if err != nil {
		log.Fatalf("GetStatus: %v", err)
	}
	if status == nil {
		fmt.Println("Status endpoint returned no payload.")
		return
	}
	fmt.Printf("Reported keys: %v\n", status.Keys)
}
