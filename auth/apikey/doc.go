// Package apikey provides API key issuance, peppered digest storage, digest-indexed validation, and rotation with grace periods.
//
// # Key lifecycle
//
//  1. Generate: create a random key with a validated prefix (e.g., "sk_live")
//  2. Digest: HMAC-SHA-256 the key with a required pepper (never store plaintext)
//  3. Validate: reject malformed keys, then resolve the key through one indexed digest lookup and compare in constant time
//  4. Rotate: issue a replacement key, set a grace period on the old key
//  5. Expire: old key stops working after the grace window
//
// # Store interface
//
// Consumers implement Store with their database, backed by a unique index on the protected digest. NewMemoryStore provides a bounded in-memory default for tests and local use.
//
//	type MyStore struct { db *gorm.DB }
//	func (s *MyStore) GetByDigest(ctx context.Context, digest string) (*apikey.Key, error) { ... }
//
// # Request authentication
//
// Manager implements auth.RequestAuthenticator and maps a validated key to a typed auth.Principal. Compose it through auth.NewChain, which parses the complete request and rejects duplicate, empty, mixed, or unsupported credentials before any lookup; transport middleware then stores the principal through authctx.
//
//	authenticator := auth.NewChain(sessions, manager)
package apikey
