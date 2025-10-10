package cstore

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
)

type mockStore struct {
	mu     sync.RWMutex
	items  map[string]*mockEntry
	hashes map[string]map[string]*mockEntry
	now    func() time.Time
}

type mockEntry struct {
	data      []byte
	etag      string
	expiresAt time.Time
}

func newMockStore() *mockStore {
	return &mockStore{
		items:  make(map[string]*mockEntry),
		hashes: make(map[string]map[string]*mockEntry),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *mockStore) seed(entries []devseed.CStoreSeedEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
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
		s.items[e.Key] = &mockEntry{
			data:      data,
			etag:      newETag(),
			expiresAt: expires,
		}
	}
	return nil
}

func (s *mockStore) getRaw(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ent, ok := s.items[key]
	if !ok {
		return []byte("null"), nil
	}
	if ent.expired(s.now()) {
		delete(s.items, key)
		return []byte("null"), nil
	}
	return append([]byte(nil), ent.data...), nil
}

func (s *mockStore) putRaw(ctx context.Context, key string, raw []byte, opts *PutOptions) (*mockEntry, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("mock cstore: key is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	ent, exists := s.items[key]
	if exists && ent.expired(now) {
		delete(s.items, key)
		exists = false
	}

	if opts != nil {
		if opts.IfAbsent && exists {
			return nil, ErrPreconditionFailed
		}
		if opts.IfETagMatch != "" {
			if !exists || ent.etag != opts.IfETagMatch {
				return nil, ErrPreconditionFailed
			}
		}
	}

	newEntry := &mockEntry{
		data: append([]byte(nil), raw...),
		etag: newETag(),
	}
	s.items[key] = newEntry
	return newEntry, nil
}

func (s *mockStore) listKeys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	keys := make([]string, 0, len(s.items))
	for key, ent := range s.items {
		if ent.expired(now) {
			delete(s.items, key)
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *mockStore) hGetRaw(ctx context.Context, hashKey, field string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.hashes[hashKey]
	if bucket == nil {
		return []byte("null"), nil
	}
	ent, ok := bucket[field]
	if !ok {
		return []byte("null"), nil
	}
	if ent.expired(s.now()) {
		delete(bucket, field)
		if len(bucket) == 0 {
			delete(s.hashes, hashKey)
		}
		return []byte("null"), nil
	}
	return append([]byte(nil), ent.data...), nil
}

func (s *mockStore) hSetRaw(ctx context.Context, hashKey, field string, raw []byte, opts *PutOptions) (*mockEntry, error) {
	if strings.TrimSpace(hashKey) == "" {
		return nil, fmt.Errorf("mock cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return nil, fmt.Errorf("mock cstore: hash field is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	bucket := s.hashes[hashKey]
	if bucket == nil {
		bucket = make(map[string]*mockEntry)
		s.hashes[hashKey] = bucket
	}

	ent, exists := bucket[field]
	if exists && ent.expired(now) {
		delete(bucket, field)
		exists = false
	}

	if opts != nil {
		if opts.IfAbsent && exists {
			return nil, ErrPreconditionFailed
		}
		if opts.IfETagMatch != "" {
			if !exists || ent.etag != opts.IfETagMatch {
				return nil, ErrPreconditionFailed
			}
		}
	}

	newEntry := &mockEntry{
		data: append([]byte(nil), raw...),
		etag: newETag(),
	}
	bucket[field] = newEntry
	return newEntry, nil
}

func (s *mockStore) hGetAllRaw(ctx context.Context, hashKey string) (map[string][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.hashes[hashKey]
	if len(bucket) == 0 {
		return nil, nil
	}

	now := s.now()
	result := make(map[string][]byte, len(bucket))
	for field, ent := range bucket {
		if ent.expired(now) {
			delete(bucket, field)
			continue
		}
		result[field] = append([]byte(nil), ent.data...)
	}
	if len(bucket) == 0 {
		delete(s.hashes, hashKey)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (e *mockEntry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

func newETag() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

func encodeHashMap(fields map[string][]byte) ([]byte, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	payload := make(map[string]json.RawMessage, len(fields))
	for field, data := range fields {
		payload[field] = json.RawMessage(append([]byte(nil), data...))
	}
	return json.Marshal(payload)
}
