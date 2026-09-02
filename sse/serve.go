package sse

import "net/http"

// ServeOption configures a single [ServeSSE] invocation: the optional
// authentication gate, per-principal client-identity resolution, and client
// metadata.
type ServeOption func(*serveConfig)

// IdentityResolver derives a client's routing key from the authenticated
// principal, enabling per-principal event scoping. It runs only after a
// successful [Authenticator] and receives the request plus the resolved
// identity. The returned clientID, when non-empty, replaces the clientID passed
// to [ServeSSE] as the broadcast-matching key; the returned options add client
// metadata. A returned [apperrors.Forbidden] error rejects the connection with
// 403, any other error with 401.
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
func WithAuthenticator(a Authenticator) ServeOption {
	return func(c *serveConfig) {
		if a != nil {
			c.authenticator = a
		}
	}
}

// WithClientIdentity injects an [IdentityResolver] that maps the authenticated
// principal to a routing key (and optional metadata) so events can be scoped
// per-principal. It has effect only when an [Authenticator] is also configured.
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
	return c
}
