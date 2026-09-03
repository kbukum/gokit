package testutil

import (
	"net/http"
	"sync/atomic"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
)

var _ sse.Authenticator = (*FakeAuthenticator)(nil)

// FakeAuthenticator is a deterministic [sse.Authenticator] for tests. It returns
// a fixed identity or a fixed rejection error and counts how many times it ran,
// so tests can assert the gate was exercised. It is safe for concurrent use.
type FakeAuthenticator struct {
	// Identity is returned on success (when Err is nil).
	Identity any
	// Err, when non-nil, rejects every connection. Use an [apperrors.Forbidden]
	// error to exercise the 403 path; any other error maps to 401.
	Err error

	calls atomic.Int64
}

// Authenticate implements [sse.Authenticator].
func (f *FakeAuthenticator) Authenticate(_ *http.Request) (any, error) {
	f.calls.Add(1)
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Identity, nil
}

// Calls reports how many times Authenticate was invoked.
func (f *FakeAuthenticator) Calls() int { return int(f.calls.Load()) }

// AllowAuthenticator returns a [FakeAuthenticator] that admits every connection
// with the given identity.
func AllowAuthenticator(identity any) *FakeAuthenticator {
	return &FakeAuthenticator{Identity: identity}
}

// RejectUnauthorized returns a [FakeAuthenticator] that rejects every connection
// with a 401 (missing/invalid credential).
func RejectUnauthorized(reason string) *FakeAuthenticator {
	return &FakeAuthenticator{Err: apperrors.Unauthorized(reason)}
}

// RejectForbidden returns a [FakeAuthenticator] that rejects every connection
// with a 403 (authenticated but not permitted).
func RejectForbidden(reason string) *FakeAuthenticator {
	return &FakeAuthenticator{Err: apperrors.Forbidden(reason)}
}
