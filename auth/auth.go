package auth

import "context"

// TokenValidator validates credentials without discarding their identity type or request cancellation.
type TokenValidator[T any] interface {
	ValidateToken(context.Context, string) (T, error)
}

// TokenValidatorFunc adapts a typed validation function.
type TokenValidatorFunc[T any] func(context.Context, string) (T, error)

// ValidateToken implements TokenValidator.
func (f TokenValidatorFunc[T]) ValidateToken(ctx context.Context, token string) (T, error) {
	return f(ctx, token)
}

// TokenGenerator creates tokens from typed claims.
type TokenGenerator[T any] interface {
	GenerateToken(context.Context, T) (string, error)
}

// TokenGeneratorFunc adapts a typed generation function.
type TokenGeneratorFunc[T any] func(context.Context, T) (string, error)

// GenerateToken implements TokenGenerator.
func (f TokenGeneratorFunc[T]) GenerateToken(ctx context.Context, claims T) (string, error) {
	return f(ctx, claims)
}

// NewValidator adapts a typed validation function.
func NewValidator[T any](fn func(context.Context, string) (T, error)) TokenValidator[T] {
	return TokenValidatorFunc[T](fn)
}
