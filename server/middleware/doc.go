// Package middleware provides reusable HTTP middlewares: authentication, rate limiting, request logging, recovery, request ID propagation, and CORS.
//
// HTTPAuth authenticates complete requests through an injected typed provider before HTTP, Connect, or SSE dispatch; with WithMissingPolicy(AcceptMissing), credential-less requests continue without an identity. Auth and OptionalAuth are header-only Gin bearer adapters with context-aware validators; optional authentication accepts missing credentials but rejects every invalid presented credential.
package middleware
