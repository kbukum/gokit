// Package apikey provides API key issuance, peppered digest storage, prefix-based lookup, validation, and rotation with grace periods.
//
// This package is framework-agnostic at its core (Hasher, Manager, Validate), and provides optional net/http middleware for request authentication.
//
// # Key lifecycle
//
//  1. Generate: create a random key with a validated prefix (e.g., "sk_live")
//  2. Digest: HMAC-SHA-256 the key with a required pepper (never store plaintext)
//  3. Validate: split the incoming key, look up by prefix, compare digests in constant time
//  4. Rotate: issue a replacement key, set a grace period on the old key
//  5. Expire: old key stops working after the grace window
//
// # Store interface
//
// Consumers implement the Store interface with their database:
//
//	type MyStore struct { db *gorm.DB }
//	func (s *MyStore) ListByPrefix(ctx context.Context, prefix string) ([]*apikey.Key, error) { ... }
//
// # Middleware
//
//	mux.Handle("/", apikey.Middleware(myValidator, apikey.WithHeader("X-API-Key"))(handler))
package apikey
