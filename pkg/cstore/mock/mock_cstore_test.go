package mock_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore/mock"
)

type sample struct {
	Value string `json:"value"`
}

func TestMockPutGetTTL(t *testing.T) {
	now := time.Now().UTC()
	m := mock.New(mock.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	ttl := 1

	if _, err := mock.Put(ctx, m, "foo", sample{Value: "fresh"}, &cstore.PutOptions{TTLSeconds: &ttl}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	item, err := mock.Get[sample](ctx, m, "foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil || item.Value.Value != "fresh" || item.ETag == "" || item.ExpiresAt == nil {
		t.Fatalf("unexpected item: %#v", item)
	}

	now = now.Add(2 * time.Second)
	expired, err := mock.Get[sample](ctx, m, "foo")
	if err != nil {
		t.Fatalf("Get expired: %v", err)
	}
	if expired != nil {
		t.Fatalf("expected nil after TTL, got %#v", expired)
	}
}

func TestMockConditionalWrites(t *testing.T) {
	now := time.Now().UTC()
	m := mock.New(mock.WithClock(func() time.Time { return now }))
	ctx := context.Background()

	item, err := mock.Put(ctx, m, "key", sample{Value: "v1"}, nil)
	if err != nil {
		t.Fatalf("Put initial: %v", err)
	}

	if _, err := mock.Put(ctx, m, "key", sample{Value: "second"}, &cstore.PutOptions{IfAbsent: true}); !errors.Is(err, cstore.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed for IfAbsent, got %v", err)
	}

	if _, err := mock.Put(ctx, m, "key", sample{Value: "second"}, &cstore.PutOptions{IfETagMatch: "wrong"}); !errors.Is(err, cstore.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed for bad ETag, got %v", err)
	}

	updated, err := mock.Put(ctx, m, "key", sample{Value: "second"}, &cstore.PutOptions{IfETagMatch: item.ETag})
	if err != nil {
		t.Fatalf("conditional Put: %v", err)
	}
	if updated.Value.Value != "second" || updated.ETag == item.ETag {
		t.Fatalf("expected value updated with new etag: %#v", updated)
	}
}

func TestMockList(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	if _, err := mock.Put(ctx, m, "jobs:1", sample{Value: "one"}, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := mock.Put(ctx, m, "jobs:2", sample{Value: "two"}, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := mock.Put(ctx, m, "logs:1", sample{Value: "ignore"}, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}

	page, err := mock.List[sample](ctx, m, "jobs:", "", 1)
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Key != "jobs:1" {
		t.Fatalf("unexpected page1: %#v", page)
	}
	if page.NextCursor == "" {
		t.Fatalf("expected next cursor")
	}

	page2, err := mock.List[sample](ctx, m, "jobs:", page.NextCursor, 5)
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].Key != "jobs:2" || page2.NextCursor != "" {
		t.Fatalf("unexpected page2: %#v", page2)
	}
}

func TestMockSeed(t *testing.T) {
	m := mock.New()
	seed := []devseed.CStoreSeedEntry{
		{Key: "hello", Value: json.RawMessage(`{"value":"world"}`)},
	}
	if err := m.Seed(seed); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	got, err := mock.Get[sample](context.Background(), m, "hello")
	if err != nil {
		t.Fatalf("Get after seed: %v", err)
	}
	if got == nil || got.Value.Value != "world" {
		t.Fatalf("unexpected seeded value: %#v", got)
	}
}
