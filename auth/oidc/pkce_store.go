package oidc

import (
	"sync"
	"time"
)

type pkceEntry struct {
	verifier  string
	expiresAt time.Time
}

// PKCEStore holds PKCE code verifiers keyed by OAuth state parameter.
// Used between AuthURL generation (where code_challenge is sent)
// and the callback (where code_verifier is needed for token exchange).
// Thread-safe with automatic expiration.
type PKCEStore struct {
	mu      sync.Mutex
	entries map[string]pkceEntry
	ttl     time.Duration
}

// NewPKCEStore creates a new PKCE store with the given TTL for entries.
func NewPKCEStore(ttl time.Duration) *PKCEStore {
	return &PKCEStore{
		entries: make(map[string]pkceEntry),
		ttl:     ttl,
	}
}

// Save stores a code verifier for the given OAuth state.
func (s *PKCEStore) Save(state, codeVerifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[state] = pkceEntry{
		verifier:  codeVerifier,
		expiresAt: time.Now().Add(s.ttl),
	}
}

// Pop retrieves and removes the code verifier for a state. Returns empty string if not found
// or expired.
func (s *PKCEStore) Pop(state string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[state]
	if !ok {
		return ""
	}
	delete(s.entries, state)
	if time.Now().After(e.expiresAt) {
		return ""
	}
	return e.verifier
}
