package connect

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	"github.com/kbukum/gokit/auth/jwt"
)

// ---------------------------------------------------------------------------
// Generic Token Authentication Interceptor
// ---------------------------------------------------------------------------

// TokenAuthInterceptor returns a Connect interceptor that validates tokens
// using any auth.TokenValidator implementation and stores the parsed claims
// in context via authctx.Set.
//
// This is the preferred way to add authentication — it works with any
// validator (JWT, OIDC, API key, etc.) without coupling to a specific implementation.
//
// Usage:
//
//	validator := jwtSvc.AsValidator() // or any auth.TokenValidator
//	path, handler := myv1connect.NewMyServiceHandler(
//	    svc,
//	    connect.WithInterceptors(
//	        TokenAuthInterceptor(validator),
//	    ),
//	)
func TokenAuthInterceptor(validator auth.TokenValidator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			header := req.Header().Get("Authorization")
			if header == "" {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("missing authorization header"),
				)
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("invalid authorization scheme; expected 'Bearer <token>'"),
				)
			}

			claims, err := validator.ValidateToken(token)
			if err != nil {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("invalid or expired token"),
				)
			}

			ctx = authctx.Set(ctx, claims)
			return next(ctx, req)
		}
	}
}

// OptionalTokenAuthInterceptor is like TokenAuthInterceptor but allows requests
// without authentication to proceed. If a valid token is present, claims are
// stored in context; otherwise, the request continues without claims.
func OptionalTokenAuthInterceptor(validator auth.TokenValidator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			header := req.Header().Get("Authorization")
			if header == "" {
				return next(ctx, req)
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				return next(ctx, req)
			}

			claims, err := validator.ValidateToken(token)
			if err != nil {
				return next(ctx, req)
			}

			ctx = authctx.Set(ctx, claims)
			return next(ctx, req)
		}
	}
}

// ---------------------------------------------------------------------------
// JWT Authentication Interceptor
// ---------------------------------------------------------------------------

// JWTAuthInterceptor returns a Connect interceptor that validates JWT tokens
// using the provided JWT service and stores the parsed claims in the request
// context using authctx.
//
// The interceptor:
//  1. Extracts the JWT from the "Authorization: Bearer <token>" header
//  2. Validates and parses the token using the JWT service
//  3. Stores the parsed claims in the context via authctx.Set
//  4. Returns CodeUnauthenticated errors for missing/invalid tokens
//
// Usage:
//
//	// Define your claims type
//	type MyClaims struct {
//	    jwt.RegisteredClaims
//	    UserID   string `json:"user_id"`
//	    TenantID string `json:"tenant_id"`
//	}
//
//	// Create JWT service
//	jwtSvc, _ := jwt.NewService(jwtCfg, func() *MyClaims { return &MyClaims{} })
//
//	// Add interceptor to Connect mux
//	mux := http.NewServeMux()
//	path, handler := myv1connect.NewMyServiceHandler(
//	    svc,
//	    connect.WithInterceptors(
//	        JWTAuthInterceptor(jwtSvc),
//	        // ... other interceptors
//	    ),
//	)
//
//	// In your handler, retrieve claims
//	claims, ok := authctx.Get[*MyClaims](ctx)
//	if !ok {
//	    return nil, errors.New("no claims in context")
//	}
//	userID := claims.UserID
func JWTAuthInterceptor[T gojwt.Claims](jwtSvc *jwt.Service[T]) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract Authorization header
			header := req.Header().Get("Authorization")
			if header == "" {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("missing authorization header"),
				)
			}

			// Extract Bearer token
			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				// No "Bearer " prefix found
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("invalid authorization scheme; expected 'Bearer <token>'"),
				)
			}

			// Parse and validate JWT
			claims, err := jwtSvc.Parse(token)
			if err != nil {
				return nil, connect.NewError(
					connect.CodeUnauthenticated,
					errors.New("invalid or expired token"),
				)
			}

			// Store claims in context
			ctx = authctx.Set(ctx, claims)

			// Proceed with authenticated context
			return next(ctx, req)
		}
	}
}

// ---------------------------------------------------------------------------
// Optional JWT Interceptor
// ---------------------------------------------------------------------------

// OptionalJWTAuthInterceptor is like JWTAuthInterceptor but allows requests
// without authentication to proceed. If a valid token is present, claims are
// stored in context; otherwise, the request continues without claims.
//
// This is useful for endpoints that work both for authenticated and
// unauthenticated users (e.g., public content with optional user features).
//
// Usage:
//
//	connect.WithInterceptors(
//	    OptionalJWTAuthInterceptor(jwtSvc),
//	)
//
//	// In handler, check if user is authenticated
//	claims, ok := authctx.Get[*MyClaims](ctx)
//	if ok {
//	    // User is authenticated
//	    userID := claims.UserID
//	} else {
//	    // Anonymous access
//	}
func OptionalJWTAuthInterceptor[T gojwt.Claims](jwtSvc *jwt.Service[T]) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract Authorization header
			header := req.Header().Get("Authorization")
			if header == "" {
				// No auth header — proceed without claims
				return next(ctx, req)
			}

			// Extract Bearer token
			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				// Invalid scheme — proceed without claims
				return next(ctx, req)
			}

			// Parse and validate JWT
			claims, err := jwtSvc.Parse(token)
			if err != nil {
				// Invalid token — proceed without claims
				return next(ctx, req)
			}

			// Store claims in context
			ctx = authctx.Set(ctx, claims)

			// Proceed with authenticated context
			return next(ctx, req)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper: Require Auth in Handlers
// ---------------------------------------------------------------------------

// RequireAuth is a helper function to retrieve claims from context in handlers.
// Returns a Connect error if claims are missing (e.g., endpoint called without
// required authentication middleware).
//
// Usage in handlers:
//
//	func (s *MyService) ProtectedMethod(ctx context.Context, req *connect.Request[myv1.MyRequest]) (*connect.Response[myv1.MyResponse], error) {
//	    claims, err := RequireAuth[*MyClaims](ctx)
//	    if err != nil {
//	        return nil, err
//	    }
//	    userID := claims.UserID
//	    // ... handle request
//	}
func RequireAuth[T any](ctx context.Context) (T, error) {
	claims, ok := authctx.Get[T](ctx)
	if !ok {
		var zero T
		return zero, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("authentication required"),
		)
	}
	return claims, nil
}

// GetAuth retrieves claims from context, returning the zero value and false
// if not present. This is useful for optional auth scenarios.
//
// Usage in handlers:
//
//	claims, ok := GetAuth[*MyClaims](ctx)
//	if ok {
//	    // User authenticated
//	    userID := claims.UserID
//	} else {
//	    // Anonymous user
//	}
func GetAuth[T any](ctx context.Context) (T, bool) {
	return authctx.Get[T](ctx)
}
