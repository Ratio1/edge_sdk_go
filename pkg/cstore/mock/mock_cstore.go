package mock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
)

type entry struct {
	data      []byte
	etag      string
	expiresAt time.Time
}

func (e *entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// Mock implements an in-memory CStore replacement with TTL and ETag semantics.
type Mock struct {
	mu     sync.RWMutex
	items  map[string]*entry
	hashes map[string]map[string]*entry
	now    func() time.Time
}

// Option configures the mock instance.
type Option func(*Mock)

// WithClock overrides the clock used for TTL bookkeeping (useful in tests).
func WithClock(fn func() time.Time) Option {
	return func(m *Mock) {
		if fn != nil {
			m.now = fn
		}
	}
}

// New creates an empty mock store.
func New(opts ...Option) *Mock {
	m := &Mock{
		items:  make(map[string]*entry),
		hashes: make(map[string]map[string]*entry),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Mock) clock() time.Time {
	if m.now == nil {
		return time.Now().UTC()
	}
	return m.now()
}

// Seed loads initial items from seed entries (typically decoded via devseed.LoadCStoreSeed).
func (m *Mock) Seed(entries []devseed.CStoreSeedEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock()
	for _, e := range entries {
		if strings.TrimSpace(e.Key) == "" {
			return fmt.Errorf("mock cstore: seed entry missing key")
		}
		data := append([]byte(nil), e.Value...)
		if len(data) == 0 {
			data = []byte("null")
		}
		var expires time.Time
		if e.TTLSeconds != nil && *e.TTLSeconds > 0 {
			expires = now.Add(time.Duration(*e.TTLSeconds) * time.Second)
		}
		m.items[e.Key] = &entry{
			data:      data,
			etag:      newETag(),
			expiresAt: expires,
		}
	}
	return nil
}

// Get retrieves a value decoded into T.
func Get[T any](ctx context.Context, store *Mock, key string) (*cstore.Item[T], error) {
	return getItem[T](ctx, store, key)
}

// Put writes a value and returns the stored item.
func Put[T any](ctx context.Context, store *Mock, key string, value T, opts *cstore.PutOptions) (*cstore.Item[T], error) {
	return putItem(ctx, store, key, value, opts)
}

// List enumerates keys matching the prefix.
func List[T any](ctx context.Context, store *Mock, prefix string, cursor string, limit int) (*cstore.ListResult[T], error) {
	return listItems[T](ctx, store, prefix, cursor, limit)
}

// HGet retrieves a hash field decoded into T.
func HGet[T any](ctx context.Context, store *Mock, hashKey, field string) (*cstore.HashItem[T], error) {
	return getHashItem[T](ctx, store, hashKey, field)
}

// HSet writes a hash field and returns the stored item.
func HSet[T any](ctx context.Context, store *Mock, hashKey, field string, value T, opts *cstore.PutOptions) (*cstore.HashItem[T], error) {
	return putHashItem(ctx, store, hashKey, field, value, opts)
}

// HGetAll retrieves all hash fields for a hash key.
func HGetAll[T any](ctx context.Context, store *Mock, hashKey string) ([]cstore.HashItem[T], error) {
	return listHashItems[T](ctx, store, hashKey)
}

func getItem[T any](ctx context.Context, store *Mock, key string) (*cstore.Item[T], error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("mock cstore: key is required")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	ent, ok := store.items[key]
	if !ok {
		return nil, nil
	}
	now := store.clock()
	if ent.expired(now) {
		delete(store.items, key)
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(ent.data, &value); err != nil {
		return nil, fmt.Errorf("mock cstore: decode value: %w", err)
	}

	var expiresPtr *time.Time
	if !ent.expiresAt.IsZero() {
		expires := ent.expiresAt
		expiresPtr = &expires
	}
	return &cstore.Item[T]{
		Key:       key,
		Value:     value,
		ETag:      ent.etag,
		ExpiresAt: expiresPtr,
	}, nil
}

func putItem[T any](ctx context.Context, store *Mock, key string, value T, opts *cstore.PutOptions) (*cstore.Item[T], error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("mock cstore: key is required")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("mock cstore: encode value: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	now := store.clock()
	ent, exists := store.items[key]
	if exists && ent.expired(now) {
		delete(store.items, key)
		exists = false
	}

	if opts != nil {
		if opts.IfAbsent && exists {
			return nil, cstore.ErrPreconditionFailed
		}
		if opts.IfETagMatch != "" {
			if !exists || ent.etag != opts.IfETagMatch {
				return nil, cstore.ErrPreconditionFailed
			}
		}
	}

	newEntry := &entry{
		data: append([]byte(nil), payload...),
		etag: newETag(),
	}
	if opts != nil && opts.TTLSeconds != nil && *opts.TTLSeconds > 0 {
		newEntry.expiresAt = now.Add(time.Duration(*opts.TTLSeconds) * time.Second)
	}
	store.items[key] = newEntry

	var expiresPtr *time.Time
	if !newEntry.expiresAt.IsZero() {
		expires := newEntry.expiresAt
		expiresPtr = &expires
	}
	return &cstore.Item[T]{
		Key:       key,
		Value:     value,
		ETag:      newEntry.etag,
		ExpiresAt: expiresPtr,
	}, nil
}

func listItems[T any](ctx context.Context, store *Mock, prefix string, cursor string, limit int) (*cstore.ListResult[T], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	now := store.clock()
	keys := make([]string, 0, len(store.items))
	for key, ent := range store.items {
		if ent.expired(now) {
			delete(store.items, key)
			continue
		}
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	start := 0
	if cursor != "" {
		idx := sort.SearchStrings(keys, cursor)
		for idx < len(keys) && keys[idx] <= cursor {
			idx++
		}
		start = idx
	}
	if start > len(keys) {
		start = len(keys)
	}

	end := len(keys)
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	items := make([]cstore.Item[T], 0, end-start)
	for _, key := range keys[start:end] {
		ent := store.items[key]
		var value T
		if err := json.Unmarshal(ent.data, &value); err != nil {
			return nil, fmt.Errorf("mock cstore: decode value: %w", err)
		}
		var expiresPtr *time.Time
		if !ent.expiresAt.IsZero() {
			expires := ent.expiresAt
			expiresPtr = &expires
		}
		items = append(items, cstore.Item[T]{
			Key:       key,
			Value:     value,
			ETag:      ent.etag,
			ExpiresAt: expiresPtr,
		})
	}

	nextCursor := ""
	if end < len(keys) && end > 0 {
		nextCursor = keys[end-1]
	}

	return &cstore.ListResult[T]{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

// Delete removes a key.
func (m *Mock) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("mock cstore: key is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[key]; !ok {
		return cstore.ErrNotFound
	}
	delete(m.items, key)
	return nil
}

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

func getHashItem[T any](ctx context.Context, store *Mock, hashKey, field string) (*cstore.HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("mock cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("mock cstore: hash field is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	bucket := store.hashes[hashKey]
	if bucket == nil {
		return nil, nil
	}
	ent, ok := bucket[field]
	if !ok {
		return nil, nil
	}
	now := store.clock()
	if ent.expired(now) {
		delete(bucket, field)
		if len(bucket) == 0 {
			delete(store.hashes, hashKey)
		}
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(ent.data, &value); err != nil {
		return nil, fmt.Errorf("mock cstore: decode hash value: %w", err)
	}

	var expiresPtr *time.Time
	if !ent.expiresAt.IsZero() {
		expires := ent.expiresAt
		expiresPtr = &expires
	}
	return &cstore.HashItem[T]{
		HashKey:   hashKey,
		Field:     field,
		Value:     value,
		ETag:      ent.etag,
		ExpiresAt: expiresPtr,
	}, nil
}

func putHashItem[T any](ctx context.Context, store *Mock, hashKey, field string, value T, opts *cstore.PutOptions) (*cstore.HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("mock cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("mock cstore: hash field is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("mock cstore: encode hash value: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	now := store.clock()
	bucket := store.hashes[hashKey]
	if bucket == nil {
		bucket = make(map[string]*entry)
		store.hashes[hashKey] = bucket
	}
	ent, exists := bucket[field]
	if exists && ent.expired(now) {
		delete(bucket, field)
		exists = false
	}

	if opts != nil {
		if opts.IfAbsent && exists {
			return nil, cstore.ErrPreconditionFailed
		}
		if opts.IfETagMatch != "" {
			if !exists || ent.etag != opts.IfETagMatch {
				return nil, cstore.ErrPreconditionFailed
			}
		}
	}

	newEntry := &entry{
		data: append([]byte(nil), payload...),
		etag: newETag(),
	}
	if opts != nil && opts.TTLSeconds != nil && *opts.TTLSeconds > 0 {
		newEntry.expiresAt = now.Add(time.Duration(*opts.TTLSeconds) * time.Second)
	}
	bucket[field] = newEntry

	var expiresPtr *time.Time
	if !newEntry.expiresAt.IsZero() {
		expires := newEntry.expiresAt
		expiresPtr = &expires
	}
	return &cstore.HashItem[T]{
		HashKey:   hashKey,
		Field:     field,
		Value:     value,
		ETag:      newEntry.etag,
		ExpiresAt: expiresPtr,
	}, nil
}

func listHashItems[T any](ctx context.Context, store *Mock, hashKey string) ([]cstore.HashItem[T], error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("mock cstore: hash key is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	bucket := store.hashes[hashKey]
	if bucket == nil {
		return nil, nil
	}

	now := store.clock()
	fields := make([]string, 0, len(bucket))
	for field, ent := range bucket {
		if ent.expired(now) {
			delete(bucket, field)
			continue
		}
		fields = append(fields, field)
	}
	if len(bucket) == 0 {
		delete(store.hashes, hashKey)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	sort.Strings(fields)

	items := make([]cstore.HashItem[T], 0, len(fields))
	for _, field := range fields {
		ent := bucket[field]
		var value T
		if err := json.Unmarshal(ent.data, &value); err != nil {
			return nil, fmt.Errorf("mock cstore: decode hash value: %w", err)
		}
		var expiresPtr *time.Time
		if !ent.expiresAt.IsZero() {
			expires := ent.expiresAt
			expiresPtr = &expires
		}
		items = append(items, cstore.HashItem[T]{
			HashKey:   hashKey,
			Field:     field,
			Value:     value,
			ETag:      ent.etag,
			ExpiresAt: expiresPtr,
		})
	}
	return items, nil
}
