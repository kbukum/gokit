package sse

import (
	"context"
	"net/http"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// Access is verified request authorization. Lifetime binds streams to an authoritative session or credential lifetime; cancellation revokes delivery without session parsing in SSE.
type Access struct {
	Principal string
	Route     string
	Lifetime  context.Context
}

// Authorizer authenticates and authorizes the requested scope before admission. Credentials belong in headers or cookies, never URLs.
type Authorizer func(*http.Request) (Access, error)

// PublicAccess explicitly exposes one public scope. All visitors share its admission principal.
func PublicAccess(principal, route string) Authorizer {
	return func(*http.Request) (Access, error) {
		return Access{Principal: principal, Route: route}, nil
	}
}

// Authenticated composes an injected authenticator with a typed authorization resolver. Neither nil wiring nor a missing identity can open the endpoint.
func Authenticated(auth Authenticator, resolve func(*http.Request, any) (Access, error)) Authorizer {
	return func(r *http.Request) (Access, error) {
		if util.IsNil(auth) || resolve == nil {
			return Access{}, apperrors.Unauthorized("")
		}
		identity, err := auth.Authenticate(r)
		if err != nil {
			return Access{}, err
		}
		if util.IsNil(identity) {
			return Access{}, apperrors.Unauthorized("")
		}
		return resolve(r.WithContext(withIdentity(r.Context(), identity)), identity)
	}
}

// HandlerConfig holds injected endpoint dependencies and positive timing budgets. Use DefaultHandlerConfig and supply Authorize and Logger.
type HandlerConfig struct {
	Authorize    Authorizer
	Logger       *logging.Logger
	Clock        util.Clock
	WriteTimeout time.Duration
	Heartbeat    time.Duration
}

// DefaultHandlerConfig supplies renewable write and heartbeat budgets, but no implicit authentication policy or logger.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{Clock: util.SystemClock{}, WriteTimeout: 10 * time.Second, Heartbeat: 30 * time.Second}
}
