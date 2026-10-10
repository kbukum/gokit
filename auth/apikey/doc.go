// Package apikey provides API key issuance, peppered digest storage, digest-indexed validation, and rotation with grace periods.
//
// # Key lifecycle
//
//  1. Generate: create a random key with a validated prefix (e.g., "sk_live")
//  2. Digest: HMAC-SHA-256 the key with a required pepper (never store plaintext)
//  3. Validate: reject malformed keys, then resolve the key through one indexed digest lookup and compare in constant time
//  4. Rotate: atomically issue a replacement and give the old key an explicit grace window (zero means none)
//  5. Revoke: end a key at once, including any remaining grace; revocation is one-way
//
// # Store interface
//
// Consumers implement Store with their database, backed by a unique index on the protected digest, and run the apikeytest contract suite against it. Store.Rotate must be one compare-and-swap so rotation cannot race revocation or another rotation. NewMemoryStore provides a bounded in-memory default for tests and local use.
//
//	type MyStore struct { db *gorm.DB }
//	func (s *MyStore) GetByDigest(ctx context.Context, digest string) (*apikey.Key, error) { ... }
//
// # Request authentication
//
// Manager implements auth.RequestAuthenticator and maps a validated key to a typed auth.Principal whose Reference is the key id; ValidateKeyID revalidates that id under the same rules. Compose it through auth.NewChain, which parses the complete request and rejects duplicate, empty, mixed, or unsupported credentials before any lookup; transport middleware then stores the principal through authctx.
//
//	authenticator := auth.NewChain(sessions, manager)
package apikey
