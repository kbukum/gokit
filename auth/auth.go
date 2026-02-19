package auth

// TokenValidator validates a token string and returns the parsed claims.
// This is the core authentication contract — middleware and interceptors
// depend on this interface rather than specific implementations (JWT, OIDC, etc.).
//
// The returned value can be any type (typically a project-specific claims struct).
// It is stored in request context via authctx.Set and retrieved with authctx.Get[T].
//
// Implementations:
//   - jwt.Service[T].AsValidator() — validates JWT tokens
//   - oidc.Verifier can be adapted via TokenValidatorFunc
//   - Projects can implement custom validators (API keys, opaque tokens, etc.)
type TokenValidator interface {
	ValidateToken(token string) (any, error)
}

// TokenValidatorFunc adapts an ordinary function to the TokenValidator interface.
// This is the simplest way to create a validator:
//
//	validator := auth.TokenValidatorFunc(func(token string) (any, error) {
//	    return myCustomValidation(token)
//	})
type TokenValidatorFunc func(token string) (any, error)

// ValidateToken implements TokenValidator.
func (f TokenValidatorFunc) ValidateToken(token string) (any, error) {
	return f(token)
}

// TokenGenerator generates a signed token from claims.
// This is the token creation contract — services use this to issue tokens
// without depending on specific signing implementations.
type TokenGenerator interface {
	GenerateToken(claims any) (string, error)
}

// TokenGeneratorFunc adapts an ordinary function to the TokenGenerator interface.
type TokenGeneratorFunc func(claims any) (string, error)

// GenerateToken implements TokenGenerator.
func (f TokenGeneratorFunc) GenerateToken(claims any) (string, error) {
	return f(claims)
}

// NewValidator creates a TokenValidator from a validation function.
// This is a convenience wrapper for TokenValidatorFunc, useful for bridging
// typed services like jwt.Service[T]:
//
//	validator := auth.NewValidator(jwtSvc.ValidatorFunc())
func NewValidator(fn func(string) (any, error)) TokenValidator {
	return TokenValidatorFunc(fn)
}
