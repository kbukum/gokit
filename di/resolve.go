package di

import (
	"context"
	"fmt"
	"net/http"

	apperr "github.com/kbukum/gokit/errors"
)

// Resolve resolves the value registered for type T (optionally qualified by [WithName]).
// The context threads cycle detection and cancellation through the resolution chain;
// pass the caller's request context, or [context.Background] at startup.
// It returns an error if nothing is registered for the key, if a factory fails, if ctx is canceled,
// or if a circular dependency is detected.
func Resolve[T any](ctx context.Context, c *Container, opts ...Option) (T, error) {
	var zero T
	if c == nil {
		return zero, errNilContainer()
	}
	o := buildOptions(opts)
	k := keyFor[T](o.name)

	v, err := c.resolveKey(ctx, k)
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, apperr.New(apperr.ErrCodeInternal,
			fmt.Sprintf("di: %s is %T, expected %s", k, v, typeName[T]()),
			http.StatusInternalServerError)
	}
	return typed, nil
}

// MustResolve is the panic-on-error twin of [Resolve]. It is reserved for application startup,
// tests, and CLI wiring where a missing dependency is a programming error;
// never call it on a request-scoped path.
func MustResolve[T any](ctx context.Context, c *Container, opts ...Option) T {
	v, err := Resolve[T](ctx, c, opts...)
	if err != nil {
		panic(err)
	}
	return v
}

// TryResolve resolves the value for type T, returning the zero value and false
// if it is not registered, does not match T, or cannot be resolved.
func TryResolve[T any](ctx context.Context, c *Container, opts ...Option) (T, bool) {
	v, err := Resolve[T](ctx, c, opts...)
	if err != nil {
		var zero T
		return zero, false
	}
	return v, true
}
