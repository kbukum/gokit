// Package authctx provides type-safe context propagation for authentication claims.
//
// It uses Go generics so each project can store and retrieve its own claims type
// without gokit knowing about the specific fields.
//
// Usage:
//
//	// Store claims (typically in middleware)
//	ctx = authctx.Set(ctx, myClaims)
//
//	// Retrieve claims (in handlers)
//	claims, ok := authctx.Get[*MyClaims](ctx)
//	claims := authctx.MustGet[*MyClaims](ctx) // panics if missing
package authctx

import (
	"context"
	"errors"
)

// contextKey is an unexported type to prevent collisions with other packages.
type contextKey struct{}

// claimsKey is the single key used to store claims in context.
var claimsKey = contextKey{}

// Set stores authentication claims in the context.
// The claims can be any type — the project defines its own claims struct.
func Set(ctx context.Context, claims any) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// Get retrieves typed authentication claims from the context.
// Returns the claims and true if found and of the correct type,
// or zero value and false otherwise.
func Get[T any](ctx context.Context) (T, bool) {
	val := ctx.Value(claimsKey)
	if val == nil {
		var zero T
		return zero, false
	}
	claims, ok := val.(T)
	return claims, ok
}

// MustGet retrieves typed authentication claims from the context.
// Panics if claims are missing or of the wrong type.
// Use in handlers where authentication middleware guarantees claims exist.
func MustGet[T any](ctx context.Context) T {
	claims, ok := Get[T](ctx)
	if !ok {
		panic("authctx: claims not found in context or wrong type")
	}
	return claims
}

// ErrNoClaims is returned when claims are not found in the context.
var ErrNoClaims = errors.New("authctx: no claims in context")

// GetOrError retrieves typed claims from the context.
// Returns ErrNoClaims if claims are missing or of the wrong type.
func GetOrError[T any](ctx context.Context) (T, error) {
	claims, ok := Get[T](ctx)
	if !ok {
		var zero T
		return zero, ErrNoClaims
	}
	return claims, nil
}
