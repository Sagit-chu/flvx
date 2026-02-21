package policy

import (
	"encoding/json"
	"sync"
	"time"
)

type idempotencyEntry struct {
	Fingerprint string
	Response    []byte
	ExpiresAt   time.Time
}

type IdempotencyStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]idempotencyEntry
}

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &IdempotencyStore{
		ttl:     ttl,
		entries: make(map[string]idempotencyEntry),
	}
}

func (s *IdempotencyStore) Lookup(toolName, idempotencyKey, fingerprint string) (response []byte, replay bool, conflict bool) {
	if s == nil {
		return nil, false, false
	}
	storageKey := toolName + "::" + idempotencyKey
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[storageKey]
	if !ok {
		return nil, false, false
	}
	if now.After(entry.ExpiresAt) {
		delete(s.entries, storageKey)
		return nil, false, false
	}
	if entry.Fingerprint != fingerprint {
		return nil, false, true
	}
	out := make([]byte, len(entry.Response))
	copy(out, entry.Response)
	return out, true, false
}

func (s *IdempotencyStore) Save(toolName, idempotencyKey, fingerprint string, response any) error {
	if s == nil {
		return nil
	}
	b, err := json.Marshal(response)
	if err != nil {
		return err
	}

	storageKey := toolName + "::" + idempotencyKey

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[storageKey] = idempotencyEntry{
		Fingerprint: fingerprint,
		Response:    b,
		ExpiresAt:   time.Now().Add(s.ttl),
	}
	return nil
}
