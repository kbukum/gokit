package middleware

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// TokenValidator validates a bearer token and returns the parsed claims. It is declared locally so the transport layer (L5) never imports the auth module (L6): any concrete validator with this method — including auth.TokenValidator — satisfies it structurally and is injected by the composing application.
type TokenValidator interface {
	// ValidateToken parses and verifies token, returning opaque claims. The
	// claims type is genuinely caller-defined, so any is the documented
	// opaque-value exception here; downstream handlers recover the concrete
	// type through the ClaimsSetter's paired getter.
	ValidateToken(token string) (any, error)
}

// PermissionChecker reports whether a subject holds a permission. Declared locally to keep L5 free of the authz module (L6); any authz.Checker satisfies it structurally.
type PermissionChecker interface {
	HasPermission(subject, permission string) bool
}

// ClaimsSetter stores validated claims on ctx, returning the derived context. It is injected rather than imported so the server never depends on the auth module's context package; pass auth/authctx.Set (or an equivalent) from the composing application. The claims value is opaque by design.
type ClaimsSetter func(ctx context.Context, claims any) context.Context

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

// WithSkipPaths skips authentication for requests whose path starts with any of the given prefixes.
func WithSkipPaths(paths ...string) AuthOption {
	return func(o *authOptions) { o.skipPaths = paths }
}

// WithHeaderName sets the header to read the token from (default: "Authorization").
func WithHeaderName(name string) AuthOption {
	return func(o *authOptions) { o.headerName = name }
}

// WithScheme sets the expected token scheme (default: "Bearer"). Set to empty string to read the raw header value without scheme parsing.
func WithScheme(scheme string) AuthOption {
	return func(o *authOptions) { o.scheme = scheme }
}

// WithQueryTokenParam enables token extraction from a URL query parameter as a fallback when the header is missing.
func WithQueryTokenParam(param string) AuthOption {
	return func(o *authOptions) { o.queryTokenParam = param }
}

// WithQueryTokenAllowedPaths sets explicit endpoint paths where query token auth is allowed.
func WithQueryTokenAllowedPaths(paths ...string) AuthOption {
	return func(o *authOptions) { o.queryTokenAllowedPaths = paths }
}

// WithQueryTokenWarningLogger configures an optional hook invoked each time a token is extracted from a query parameter. Use this for audit logging when query-token auth is a fallback rather than the primary mechanism.
func WithQueryTokenWarningLogger(fn QueryTokenWarningFunc) AuthOption {
	return func(o *authOptions) { o.queryTokenWarningLogger = fn }
}

// Auth returns a Gin middleware that validates tokens and stores the parsed claims in the request context.
//
// It applies [RejectMissing]: requests without credentials are rejected with 401.
// A present-but-invalid token is always rejected. setClaims is the injected sink
// for validated claims (typically auth/authctx.Set), keeping the transport layer
// decoupled from the auth module. Both validator and setClaims must be non-nil.
func Auth(validator TokenValidator, setClaims ClaimsSetter, opts ...AuthOption) (gin.HandlerFunc, error) {
	return newAuthHandler("Auth", validator, setClaims, RejectMissing, opts...)
}

// OptionalAuth validates a token if present but allows unauthenticated requests (no Authorization header / empty token) to proceed. It applies [AcceptMissing]. A *present but invalid* token is always rejected with 401 — this is a deliberate secure-by-default contract: callers that want pass-through-on-failure should use no auth middleware at all.
//
// setClaims is the injected sink for validated claims. Both validator and setClaims must be non-nil.
func OptionalAuth(validator TokenValidator, setClaims ClaimsSetter, opts ...AuthOption) (gin.HandlerFunc, error) {
	return newAuthHandler("OptionalAuth", validator, setClaims, AcceptMissing, opts...)
}

// newAuthHandler builds the token-authentication middleware shared by Auth and OptionalAuth. policy governs only the missing-credential case; an invalid token is always rejected.
func newAuthHandler(name string, validator TokenValidator, setClaims ClaimsSetter, policy MissingTokenPolicy, opts ...AuthOption) (gin.HandlerFunc, error) {
	if validator == nil {
		return nil, fmt.Errorf("middleware/auth: %s requires a non-nil TokenValidator", name)
	}
	if setClaims == nil {
		return nil, fmt.Errorf("middleware/auth: %s requires a non-nil ClaimsSetter", name)
	}
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
			if policy == AcceptMissing {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		ctx := setClaims(c.Request.Context(), claims)
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
			// Strip the token from the URL so it is not logged, cached, or forwarded.
			q := c.Request.URL.Query()
			q.Del(o.queryTokenParam)
			c.Request.URL.RawQuery = q.Encode()
			return token, true
		}
	}

	return "", false
}
