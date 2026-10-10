package session

import (
	"context"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

// Record is protected persistence material. Never persist plaintext session credentials.
type Record struct {
	Reference   string
	Family      string
	Generation  uint64
	Principal   auth.Principal
	ExpiresAt   time.Time
	RetainUntil time.Time
	// AuthenticatedAt is when the family's user last proved a credential: set by create and relogin, kept by
	// rotation. Zero means unknown, which callers must treat as not recent.
	AuthenticatedAt time.Time
	Active          bool
	Revoked         bool
}

// Store operations are transactional and cancellation-aware. Lookup must read authoritative writer state.
// Rotate atomically compares the active generation and inserts its successor without extending expiry or changing
// the authentication time.
// Relogin compares the active, non-revoked generation and replaces its identity after password verification.
// It preserves unexpired absolute expiry; an expired active generation may start a new one-hour lifetime. It records
// the new authentication time.
// Lookup reports a missing protected reference with NOT_FOUND, never an unavailable-store error.
// LookupBatch accepts at most MaxLookupBatch distinct references; omitted entries are authoritative absence, not a store outage.
// RevokeSubject atomically revokes matching families and returns the newly transitioned family count.
// Revoke resolves even an inactive generation and revokes its entire family.
type Store interface {
	Create(context.Context, Record) error
	Lookup(context.Context, string) (Record, error)
	LookupBatch(context.Context, []string) (map[string]Record, error)
	Rotate(context.Context, string, Record) error
	Relogin(context.Context, string, Record) error
	Revoke(context.Context, string) (string, error)
	RevokeSubject(context.Context, auth.Kind, string) (int64, error)
	Cleanup(context.Context, time.Time, int) (int64, error)
}

// Issued contains the one-time plaintext credential, which callers must not log or persist.
type Issued struct {
	Token     string
	Principal auth.Principal
}

// MaxLookupBatch bounds the authority query and result payload.
const MaxLookupBatch = 256

// ValidateReferences validates a batch before persistence access.
func ValidateReferences(refs []string) error {
	if len(refs) < 1 || len(refs) > MaxLookupBatch {
		return apperrors.InvalidInput("session.references", "batch must contain 1..256 references")
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref == "" || len(ref) > 512 {
			return apperrors.InvalidInput("session.reference", "reference must contain 1..512 bytes")
		}
		if _, exists := seen[ref]; exists {
			return apperrors.InvalidInput("session.references", "references must be distinct")
		}
		seen[ref] = struct{}{}
	}
	return nil
}
