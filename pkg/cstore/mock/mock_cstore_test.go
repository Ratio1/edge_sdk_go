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

func TestMockSetGetTTL(t *testing.T) {
	now := time.Now().UTC()
	m := mock.New(mock.WithClock(func() time.Time { return now }))
	ctx := context.Background()
	ttl := 1

	if _, err := mock.Set(ctx, m, "foo", sample{Value: "fresh"}, &cstore.SetOptions{TTLSeconds: &ttl}); err != nil {
		t.Fatalf("Set: %v", err)
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

	item, err := mock.Set(ctx, m, "key", sample{Value: "v1"}, nil)
	if err != nil {
		t.Fatalf("Set initial: %v", err)
	}

	if _, err := mock.Set(ctx, m, "key", sample{Value: "second"}, &cstore.SetOptions{IfAbsent: true}); !errors.Is(err, cstore.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed for IfAbsent, got %v", err)
	}

	if _, err := mock.Set(ctx, m, "key", sample{Value: "second"}, &cstore.SetOptions{IfETagMatch: "wrong"}); !errors.Is(err, cstore.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed for bad ETag, got %v", err)
	}

	updated, err := mock.Set(ctx, m, "key", sample{Value: "second"}, &cstore.SetOptions{IfETagMatch: item.ETag})
	if err != nil {
		t.Fatalf("conditional Set: %v", err)
	}
	if updated.Value.Value != "second" || updated.ETag == item.ETag {
		t.Fatalf("expected value updated with new etag: %#v", updated)
	}
}

func TestMockGetStatus(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	if _, err := mock.Set(ctx, m, "jobs:2", sample{Value: "two"}, nil); err != nil {
		t.Fatalf("Set jobs:2: %v", err)
	}
	if _, err := mock.Set(ctx, m, "jobs:1", sample{Value: "one"}, nil); err != nil {
		t.Fatalf("Set jobs:1: %v", err)
	}

	status, err := mock.GetStatus(ctx, m)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status == nil {
		t.Fatalf("expected non-nil status")
	}
	want := []string{"jobs:1", "jobs:2"}
	if len(status.Keys) != len(want) {
		t.Fatalf("unexpected keys: %#v", status.Keys)
	}
	for i, key := range want {
		if status.Keys[i] != key {
			t.Fatalf("GetStatus mismatch at %d: got %q want %q", i, status.Keys[i], key)
		}
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

func TestMockHashOperations(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	item, err := mock.HSet(ctx, m, "jobs", "123", sample{Value: "one"}, nil)
	if err != nil {
		t.Fatalf("HSet initial: %v", err)
	}
	if item.HashKey != "jobs" || item.Field != "123" || item.Value.Value != "one" || item.ETag == "" {
		t.Fatalf("unexpected HSet result: %#v", item)
	}

	got, err := mock.HGet[sample](ctx, m, "jobs", "123")
	if err != nil {
		t.Fatalf("HGet: %v", err)
	}
	if got == nil || got.Value.Value != "one" {
		t.Fatalf("unexpected HGet result: %#v", got)
	}

	if _, err := mock.HSet(ctx, m, "jobs", "123", sample{Value: "second"}, &cstore.SetOptions{IfAbsent: true}); !errors.Is(err, cstore.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed for hash IfAbsent, got %v", err)
	}

	updated, err := mock.HSet(ctx, m, "jobs", "123", sample{Value: "second"}, &cstore.SetOptions{IfETagMatch: item.ETag})
	if err != nil {
		t.Fatalf("conditional HSet: %v", err)
	}
	if updated.Value.Value != "second" || updated.ETag == item.ETag {
		t.Fatalf("expected updated hash value with new etag: %#v", updated)
	}

	all, err := mock.HGetAll[sample](ctx, m, "jobs")
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	if len(all) != 1 || all[0].Field != "123" || all[0].Value.Value != "second" {
		t.Fatalf("unexpected HGetAll result: %#v", all)
	}

	missing, err := mock.HGet[sample](ctx, m, "jobs", "999")
	if err != nil {
		t.Fatalf("HGet missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing hash field, got %#v", missing)
	}

	empty, err := mock.HGetAll[sample](ctx, m, "unknown")
	if err != nil {
		t.Fatalf("HGetAll missing: %v", err)
	}
	if empty != nil {
		t.Fatalf("expected nil for missing hash, got %#v", empty)
	}
}
