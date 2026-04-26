package middleware

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	"github.com/kbukum/gokit/authz"
)

// QueryTokenWarningFunc logs a warning whenever query-token authentication is used.
type QueryTokenWarningFunc func(c *gin.Context, tokenParam string)

// AuthOption configures the Auth middleware.
type AuthOption func(*authOptions)

type authOptions struct {
	skipPaths               []string
	headerName              string
	scheme                  string
	queryTokenParam         string
	queryTokenAllowedPaths  []string
	queryTokenWarningLogger QueryTokenWarningFunc
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

// WithQueryTokenParam enables token extraction from a URL query parameter
// as a fallback when the header is missing.
func WithQueryTokenParam(param string) AuthOption {
	return func(o *authOptions) { o.queryTokenParam = param }
}

// WithQueryTokenAllowedPaths sets explicit endpoint paths where query token auth is allowed.
func WithQueryTokenAllowedPaths(paths ...string) AuthOption {
	return func(o *authOptions) { o.queryTokenAllowedPaths = paths }
}

// WithQueryTokenWarningLogger configures an optional hook invoked each time a
// token is extracted from a query parameter. Use this for audit logging when
// query-token auth is a fallback rather than the primary mechanism.
func WithQueryTokenWarningLogger(fn QueryTokenWarningFunc) AuthOption {
	return func(o *authOptions) { o.queryTokenWarningLogger = fn }
}

// Auth returns a Gin middleware that validates tokens and stores the parsed claims in the request context.
func Auth(validator auth.TokenValidator, opts ...AuthOption) (gin.HandlerFunc, error) {
	o := buildAuthOptions(opts...)
	if err := o.validateQueryTokenConfig(); err != nil {
		return nil, err
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, skip := range o.skipPaths {
			if strings.HasPrefix(path, skip) {
				c.Next()
				return
			}
		}

		token, ok := extractToken(c, o)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		ctx := authctx.Set(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}, nil
}

// OptionalAuth validates a token if present but allows unauthenticated requests
// (no Authorization header / empty token) to proceed. A *present but invalid* token
// is always rejected with 401 — this is a deliberate secure-by-default contract:
// callers that want pass-through-on-failure should use no auth middleware at all.
func OptionalAuth(validator auth.TokenValidator, opts ...AuthOption) (gin.HandlerFunc, error) {
	o := buildAuthOptions(opts...)
	if err := o.validateQueryTokenConfig(); err != nil {
		return nil, err
	}

	return func(c *gin.Context) {
		token, ok := extractToken(c, o)
		if !ok {
			c.Next()
			return
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		ctx := authctx.Set(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
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

// RequirePermission is a guard middleware that uses an authz.Checker.
func RequirePermission(checker authz.Checker, required string, subjectExtractor func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := subjectExtractor(c)
		if !checker.HasPermission(subject, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}

func buildAuthOptions(opts ...AuthOption) *authOptions {
	o := &authOptions{headerName: "Authorization", scheme: "Bearer"}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *authOptions) validateQueryTokenConfig() error {
	if o.queryTokenParam == "" {
		return nil
	}
	if len(o.queryTokenAllowedPaths) == 0 {
		return fmt.Errorf("middleware/auth: query token extraction requires explicit WithQueryTokenAllowedPaths")
	}
	return nil
}

// extractToken reads the token from the request based on options.
func extractToken(c *gin.Context, o *authOptions) (string, bool) {
	header := c.GetHeader(o.headerName)
	if header != "" {
		if o.scheme == "" {
			return header, true
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], o.scheme) {
			return parts[1], true
		}
	}

	if o.queryTokenParam != "" && slices.Contains(o.queryTokenAllowedPaths, c.Request.URL.Path) {
		if token := c.Query(o.queryTokenParam); token != "" {
			if o.queryTokenWarningLogger != nil {
				o.queryTokenWarningLogger(c, o.queryTokenParam)
			}
			return token, true
		}
	}

	return "", false
}
