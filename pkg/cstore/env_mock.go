package cstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Ratio1/ratio1_sdk_go/internal/devseed"
)

type mockStore struct {
	mu     sync.RWMutex
	items  map[string][]byte
	hashes map[string]map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		items:  make(map[string][]byte),
		hashes: make(map[string]map[string][]byte),
	}
}

func (s *mockStore) seed(entries []devseed.CStoreSeedEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		if strings.TrimSpace(e.Key) == "" {
			return fmt.Errorf("mock cstore: seed entry missing key")
		}
		data := append([]byte(nil), e.Value...)
		if len(data) == 0 {
			data = []byte("null")
		}
		s.items[e.Key] = data
	}
	return nil
}

func (s *mockStore) get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.items[key]
	if !ok {
		return []byte("null"), nil
	}
	return append([]byte(nil), data...), nil
}

func (s *mockStore) set(ctx context.Context, key string, raw []byte, opts *SetOptions) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("mock cstore: key is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = append([]byte(nil), raw...)
	return nil
}

func (s *mockStore) status(ctx context.Context) (*Status, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.items) == 0 {
		return &Status{Keys: nil}, nil
	}

	keys := make([]string, 0, len(s.items))
	for key := range s.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return &Status{Keys: keys}, nil
}

func (s *mockStore) hGet(ctx context.Context, hashKey, field string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket := s.hashes[hashKey]
	if bucket == nil {
		return []byte("null"), nil
	}
	data, ok := bucket[field]
	if !ok {
		return []byte("null"), nil
	}
	return append([]byte(nil), data...), nil
}

func (s *mockStore) hSet(ctx context.Context, hashKey, field string, raw []byte, opts *SetOptions) error {
	if strings.TrimSpace(hashKey) == "" {
		return fmt.Errorf("mock cstore: hash key is required")
	}
	if strings.TrimSpace(field) == "" {
		return fmt.Errorf("mock cstore: hash field is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.hashes[hashKey]
	if bucket == nil {
		bucket = make(map[string][]byte)
		s.hashes[hashKey] = bucket
	}
	bucket[field] = append([]byte(nil), raw...)
	return nil
}

func (s *mockStore) hGetAll(ctx context.Context, hashKey string) (map[string][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket := s.hashes[hashKey]
	if len(bucket) == 0 {
		return nil, nil
	}

	result := make(map[string][]byte, len(bucket))
	for field, data := range bucket {
		result[field] = append([]byte(nil), data...)
	}
	return result, nil
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
