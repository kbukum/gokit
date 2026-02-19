package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	"github.com/kbukum/gokit/authz"
)

// AuthOption configures the Auth middleware.
type AuthOption func(*authOptions)

type authOptions struct {
	skipPaths  []string
	headerName string
	scheme     string
}

// WithSkipPaths skips authentication for requests whose path starts with
// any of the given prefixes.
func WithSkipPaths(paths ...string) AuthOption {
	return func(o *authOptions) { o.skipPaths = paths }
}

// WithHeaderName sets the header to read the token from (default: "Authorization").
func WithHeaderName(name string) AuthOption {
	return func(o *authOptions) { o.headerName = name }
}

// WithScheme sets the expected token scheme (default: "Bearer").
// Set to empty string to read the raw header value without scheme parsing.
func WithScheme(scheme string) AuthOption {
	return func(o *authOptions) { o.scheme = scheme }
}

// Auth returns a Gin middleware that validates tokens and stores the parsed
// claims in the request context. Claims are retrieved in handlers with:
//
//	claims, ok := authctx.Get[*MyClaims](c.Request.Context())
//
// The validator is any auth.TokenValidator implementation — JWT, OIDC, API key, etc.
// For JWT, use jwtService.AsValidator().
func Auth(validator auth.TokenValidator, opts ...AuthOption) gin.HandlerFunc {
	o := &authOptions{headerName: "Authorization", scheme: "Bearer"}
	for _, opt := range opts {
		opt(o)
	}

	return func(c *gin.Context) {
		// Skip configured paths
		path := c.Request.URL.Path
		for _, skip := range o.skipPaths {
			if strings.HasPrefix(path, skip) {
				c.Next()
				return
			}
		}

		// Extract token
		token, ok := extractToken(c, o)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization required",
			})
			return
		}

		// Validate
		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			return
		}

		// Store claims in request context (type-safe retrieval via authctx.Get[T])
		ctx := authctx.Set(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// OptionalAuth validates a token if present but allows unauthenticated requests.
// If valid, claims are stored in context. If absent or invalid, request proceeds.
func OptionalAuth(validator auth.TokenValidator, opts ...AuthOption) gin.HandlerFunc {
	o := &authOptions{headerName: "Authorization", scheme: "Bearer"}
	for _, opt := range opts {
		opt(o)
	}

	return func(c *gin.Context) {
		token, ok := extractToken(c, o)
		if !ok {
			c.Next()
			return
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.Next()
			return
		}

		ctx := authctx.Set(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Require is a generic guard middleware. It calls the check function and
// returns 403 Forbidden if the check returns false.
//
// This is the most flexible guard — use it for any authorization logic:
//
//	router.Use(middleware.Require(func(c *gin.Context) bool {
//	    claims, ok := authctx.Get[*MyClaims](c.Request.Context())
//	    return ok && claims.IsAdmin()
//	}))
func Require(check func(c *gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !check(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions",
			})
			return
		}
		c.Next()
	}
}

// RequirePermission is a guard middleware that uses an authz.Checker.
// The subjectExtractor reads the subject (e.g., role name) from the request,
// and the checker determines if the subject has the required permission.
//
// Example:
//
//	checker := authz.NewMapChecker(rolePermissions)
//	router.Use(middleware.RequirePermission(
//	    checker,
//	    "article:write",
//	    func(c *gin.Context) string {
//	        claims, _ := authctx.Get[*MyClaims](c.Request.Context())
//	        return claims.Role
//	    },
//	))
func RequirePermission(checker authz.Checker, required string, subjectExtractor func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := subjectExtractor(c)
		if !checker.HasPermission(subject, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions",
			})
			return
		}
		c.Next()
	}
}

// extractToken reads the token from the request based on options.
func extractToken(c *gin.Context, o *authOptions) (string, bool) {
	header := c.GetHeader(o.headerName)
	if header == "" {
		return "", false
	}

	// If no scheme expected, return raw header value
	if o.scheme == "" {
		return header, true
	}

	// Parse "Scheme token" format
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], o.scheme) {
		return "", false
	}

	return parts[1], true
}
