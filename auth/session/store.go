package session

import (
	"context"
	"time"

	"github.com/kbukum/gokit/auth"
)

// Record is protected persistence material. Never persist plaintext session credentials.
type Record struct {
	Reference   string
	Family      string
	Generation  uint64
	Principal   auth.Principal
	ExpiresAt   time.Time
	RetainUntil time.Time
	Active      bool
	Revoked     bool
}

// Store operations are transactional and cancellation-aware. Lookup must read authoritative writer state.
// Rotate atomically compares the active generation and inserts its successor without extending expiry.
// Relogin compares the active, non-revoked generation and replaces its identity after password verification.
// It preserves unexpired absolute expiry; an expired active generation may start a new one-hour lifetime.
// Lookup reports a missing protected reference with NOT_FOUND, never an unavailable-store error.
// Revoke resolves even an inactive generation and revokes its entire family.
type Store interface {
	Create(context.Context, Record) error
	Lookup(context.Context, string) (Record, error)
	Rotate(context.Context, string, Record) error
	Relogin(context.Context, string, Record) error
	Revoke(context.Context, string) (string, error)
	Cleanup(context.Context, time.Time, int) (int64, error)
}

// Issued contains the one-time plaintext credential, which callers must not log or persist.
type Issued struct {
	Token     string
	Principal auth.Principal
}
