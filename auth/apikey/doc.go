// Package apikey provides generic API key generation, hashing, validation,
// and rotation with grace periods.
//
// This package is framework-agnostic at its core (Generate, Hash, Validate, Rotate),
// and provides optional net/http middleware for request authentication.
//
// # Key lifecycle
//
//  1. Generate: create a random key with a configurable prefix (e.g., "sk_live_")
//  2. Hash: SHA-256 hash the key for storage (never store plaintext)
//  3. Validate: on each request, hash the incoming key, look up by hash, check expiry
//  4. Rotate: generate a new key, set a grace period on the old key
//  5. Expire: old key stops working after the grace window
//
// # Store interface
//
// Consumers implement the Store interface with their database:
//
//	type MyStore struct { db *gorm.DB }
//	func (s *MyStore) GetByHash(ctx context.Context, hash string) (*apikey.Key, error) { ... }
//
// # Middleware
//
//	mux.Handle("/", apikey.Middleware(myValidator, apikey.WithHeader("X-API-Key"))(handler))
package apikey
