package mock_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore/mock"
)

type sample struct {
	Value string `json:"value"`
}

func TestMockSetAndGet(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	if err := mock.Set(ctx, m, "foo", sample{Value: "bar"}, nil); err != nil {
		t.Fatalf("Set: %v", err)
	}

	item, err := mock.Get[sample](ctx, m, "foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item == nil || item.Value.Value != "bar" {
		t.Fatalf("unexpected item: %#v", item)
	}

	missing, err := mock.Get[sample](ctx, m, "missing")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing key, got %#v", missing)
	}
}

func TestMockGetStatus(t *testing.T) {
	m := mock.New()
	ctx := context.Background()

	if err := mock.Set(ctx, m, "jobs:2", sample{Value: "two"}, nil); err != nil {
		t.Fatalf("Set jobs:2: %v", err)
	}
	if err := mock.Set(ctx, m, "jobs:1", sample{Value: "one"}, nil); err != nil {
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

	if err := mock.HSet(ctx, m, "jobs", "123", sample{Value: "one"}, nil); err != nil {
		t.Fatalf("HSet initial: %v", err)
	}

	got, err := mock.HGet[sample](ctx, m, "jobs", "123")
	if err != nil {
		t.Fatalf("HGet: %v", err)
	}
	if got == nil || got.Value.Value != "one" {
		t.Fatalf("unexpected HGet result: %#v", got)
	}

	if err := mock.HSet(ctx, m, "jobs", "123", sample{Value: "second"}, nil); err != nil {
		t.Fatalf("HSet update: %v", err)
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
}
