package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// TokenValidator validates a bearer credential using the request context. Authentication providers satisfy this local contract without an upward transport dependency.
type TokenValidator[T any] interface {
	ValidateToken(context.Context, string) (T, error)
}

// ClaimsSetter stores a typed identity in a derived request context. Inject the authentication provider's paired setter and getter at composition time.
type ClaimsSetter[T any] func(context.Context, T) context.Context

// PermissionChecker reports whether a subject holds a permission without importing an authorization implementation.
type PermissionChecker interface {
	HasPermission(subject, permission string) bool
}

// AuthOption configures bearer authentication.
type AuthOption func(*authOptions)

type authOptions struct {
	skipPaths []string
}

// WithSkipPaths explicitly bypasses authentication for the supplied path prefixes.
func WithSkipPaths(paths ...string) AuthOption {
	return func(o *authOptions) { o.skipPaths = append([]string(nil), paths...) }
}

// Auth requires a valid bearer header and stores its typed identity. Cookie/session authentication belongs in HTTPAuth, where the injected authenticator sees the complete request.
func Auth[T any](validator TokenValidator[T], setClaims ClaimsSetter[T], opts ...AuthOption) (gin.HandlerFunc, error) {
	return newAuthHandler(validator, setClaims, RejectMissing, opts...)
}

// OptionalAuth accepts a missing Authorization header. Every presented but malformed or invalid credential is rejected; query parameters are never credentials.
func OptionalAuth[T any](validator TokenValidator[T], setClaims ClaimsSetter[T], opts ...AuthOption) (gin.HandlerFunc, error) {
	return newAuthHandler(validator, setClaims, AcceptMissing, opts...)
}

func newAuthHandler[T any](validator TokenValidator[T], setClaims ClaimsSetter[T], policy MissingTokenPolicy, opts ...AuthOption) (gin.HandlerFunc, error) {
	if util.IsNil(validator) {
		return nil, apperrors.InvalidInput("validator", "a non-nil TokenValidator is required")
	}
	if setClaims == nil {
		return nil, apperrors.InvalidInput("setClaims", "a non-nil ClaimsSetter is required")
	}
	o := &authOptions{}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.InvalidInput("options", "authentication options must not be nil")
		}
		opt(o)
	}
	return func(c *gin.Context) {
		for _, skip := range o.skipPaths {
			if strings.HasPrefix(c.Request.URL.Path, skip) {
				c.Next()
				return
			}
		}
		token, present, err := security.ParseBearerHeader(c.Request.Header)
		if !present && err == nil && policy == AcceptMissing {
			c.Next()
			return
		}
		if err != nil || !present {
			c.Abort()
			c.Header("WWW-Authenticate", security.BearerAuthScheme)
			WriteProblemDetails(c.Writer, c.Request, apperrors.Unauthorized(""))
			return
		}
		claims, err := validator.ValidateToken(c.Request.Context(), token)
		if err != nil || util.IsNil(claims) {
			c.Abort()
			c.Header("WWW-Authenticate", security.BearerAuthScheme)
			WriteProblemDetails(c.Writer, c.Request, apperrors.Unauthorized("").WithCause(err))
			return
		}
		c.Request = c.Request.WithContext(setClaims(c.Request.Context(), claims))
		c.Next()
	}, nil
}

// Require is a generic guard middleware.
func Require(check func(c *gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !check(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}

// RequirePermission is a guard middleware that uses a PermissionChecker.
func RequirePermission(checker PermissionChecker, required string, subjectExtractor func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := subjectExtractor(c)
		if !checker.HasPermission(subject, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
