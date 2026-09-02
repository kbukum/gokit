package sse

import "net/http"

// ServeOption configures a single [ServeSSE] invocation: the optional
// authentication gate, per-principal client-identity resolution, and client
// metadata.
type ServeOption func(*serveConfig)

// IdentityResolver derives a client's routing key from the authenticated
// principal, enabling per-principal event scoping. It runs only after a
// successful [Authenticator] and receives the request plus the resolved
// identity. The returned clientID, when non-empty, becomes the client's
// broadcast-matching route (via [WithRoute]) without replacing the unique
// per-connection id, so concurrent streams for one principal coexist; the
// returned options add client metadata. A returned [apperrors.Forbidden] error
// rejects the connection with 403, any other error with 401.
type IdentityResolver func(r *http.Request, identity any) (clientID string, opts []ClientOption, err error)

type serveConfig struct {
	authenticator Authenticator
	resolver      IdentityResolver
	clientOpts    []ClientOption
}

// WithAuthenticator injects an [Authenticator] that gates the connection before
// the stream opens. On rejection the handler writes the mapped 401/403 status and
// never starts the event loop. Without this option the endpoint stays
// unauthenticated.
//
// A nil authenticator is a wiring error, not an "unauthenticated" request: it is
// installed as a fail-closed gate that rejects every connection with 401, so an
// endpoint meant to be protected can never silently open up.
func WithAuthenticator(a Authenticator) ServeOption {
	// Normalize before capturing so the returned option is read-only: a
	// ServeOption is commonly built once and applied by many concurrent
	// handlers, and mutating the captured value inside the closure would race.
	if a == nil {
		a = failClosedAuthenticator{}
	}
	return func(c *serveConfig) {
		c.authenticator = a
	}
}

// WithClientIdentity injects an [IdentityResolver] that maps the authenticated
// principal to a routing key (and optional metadata) so events can be scoped
// per-principal. A resolver runs only after a successful [Authenticator], so
// configuring one without [WithAuthenticator] is a wiring error, not an
// unauthenticated endpoint: the connection then fails closed (rejected with 401)
// rather than silently serving an intended per-principal endpoint to anyone.
func WithClientIdentity(fn IdentityResolver) ServeOption {
	return func(c *serveConfig) {
		if fn != nil {
			c.resolver = fn
		}
	}
}

// WithClientOptions applies client metadata options (such as [WithUserID],
// [WithSessionID], or [WithMetadata]) to the connecting client.
func WithClientOptions(opts ...ClientOption) ServeOption {
	return func(c *serveConfig) {
		c.clientOpts = append(c.clientOpts, opts...)
	}
}

func newServeConfig(opts ...ServeOption) *serveConfig {
	c := &serveConfig{}
	for _, opt := range opts {
		opt(c)
	}
	// A resolver only makes sense after authentication. If one was supplied
	// without an authenticator, fail closed rather than serve an intended
	// per-principal endpoint unauthenticated.
	if c.resolver != nil && c.authenticator == nil {
		c.authenticator = failClosedAuthenticator{}
	}
	return c
}
