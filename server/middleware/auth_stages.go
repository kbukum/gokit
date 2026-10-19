package middleware

import (
	"context"
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// AuthStages splits complete-request authentication so that one result decides both access and trace trust. [AuthStages.Authenticate] runs the authenticator once, outside tracing, and records the outcome without writing a response; [AuthStages.Trusted] reads that outcome for [TracingConfig].TrustParent; [AuthStages.Enforce], inside tracing, admits or rejects by it, so a rejection carries the request's trace ID. A rejected or anonymous request is untrusted and gets a fresh root trace. Each AuthStages value records under its own key, so stages of different authenticators never read each other's outcomes.
type AuthStages[T any] struct {
	auth      Authenticator[T]
	setClaims ClaimsSetter[T]
	options   httpAuthOptions
	key       *authOutcomeKey
}

// authOutcomeKey is not zero-sized, so each AuthStages gets a distinct pointer.
type authOutcomeKey struct{ _ byte }

type authOutcome[T any] struct {
	identity T
	present  bool
	err      error
}

// NewAuthStages builds the stages from the same arguments as [HTTPAuth].
func NewAuthStages[T any](auth Authenticator[T], setClaims ClaimsSetter[T], opts ...HTTPAuthOption) (*AuthStages[T], error) {
	if util.IsNil(auth) {
		return nil, apperrors.InvalidInput("authenticator", "a non-nil Authenticator is required")
	}
	if setClaims == nil {
		return nil, apperrors.InvalidInput("setClaims", "a non-nil ClaimsSetter is required")
	}
	o := httpAuthOptions{writeError: WriteProblemDetails}
	for _, option := range opts {
		if option == nil {
			return nil, apperrors.InvalidInput("options", "authentication options must not be nil")
		}
		option(&o)
	}
	if o.writeError == nil {
		return nil, apperrors.InvalidInput("writeError", "a non-nil AuthErrorWriter is required")
	}
	if o.missing != RejectMissing && o.missing != AcceptMissing {
		return nil, apperrors.InvalidInput("missing", "unknown missing-credential policy")
	}
	return &AuthStages[T]{auth: auth, setClaims: setClaims, options: o, key: &authOutcomeKey{}}, nil
}

// Authenticate records the request's authentication outcome and always dispatches. Install it outside tracing, for example with server.Server.Authenticate.
func (s *AuthStages[T]) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), s.key, s.authenticate(r))))
	})
}

// Trusted reports whether the recorded outcome admitted an identity. A request without a recorded outcome is untrusted.
func (s *AuthStages[T]) Trusted(r *http.Request) bool {
	out, ok := r.Context().Value(s.key).(authOutcome[T])
	return ok && out.err == nil && out.present
}

// Enforce rejects a request whose outcome failed with the configured writer, or dispatches it with the identity stored. Without a recorded outcome it authenticates the request itself, so it never admits an unauthenticated request.
func (s *AuthStages[T]) Enforce(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, ok := r.Context().Value(s.key).(authOutcome[T])
		if !ok {
			out = s.authenticate(r)
		}
		if out.err == nil && !out.present && s.options.missing == AcceptMissing {
			next.ServeHTTP(w, r)
			return
		}
		if out.err != nil {
			w.Header().Set("Cache-Control", "no-store")
			s.options.writeError(w, r, out.err)
			return
		}
		next.ServeHTTP(w, r.WithContext(s.setClaims(r.Context(), out.identity)))
	})
}

func (s *AuthStages[T]) authenticate(r *http.Request) authOutcome[T] {
	identity, present, err := s.auth.Authenticate(r)
	if err == nil && present && util.IsNil(identity) {
		err = apperrors.Unauthorized("")
	}
	if err == nil && !present && s.options.missing != AcceptMissing {
		err = apperrors.Unauthorized("")
	}
	return authOutcome[T]{identity: identity, present: present, err: err}
}
