package mcp

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// StreamableHTTPConfig configures a hardened MCP Streamable HTTP handler. Localhost (loopback) protection is enabled by default and Origin values are validated up front; both fail closed.
type StreamableHTTPConfig struct {
	// Stateless serves each request without persistent session state.
	Stateless bool
	// JSONResponse forces buffered JSON responses instead of SSE streams.
	JSONResponse bool
	// Logger receives SDK handler activity.
	Logger *slog.Logger
	// EventStore optionally persists SSE events for resumability.
	EventStore sdkmcp.EventStore
	// SessionTimeout bounds idle session lifetime.
	SessionTimeout time.Duration
	// AllowedOrigins is the exact set of browser Origins permitted via the returned CrossOriginProtection. Each is validated and normalized.
	AllowedOrigins []string
	// DisableLocalhostProtection turns off the SDK's loopback bind guard. Leave false unless the handler is intentionally exposed beyond localhost.
	DisableLocalhostProtection bool
}

// NewStreamableHTTPOptions builds SDK Streamable HTTP options plus a CrossOriginProtection preloaded with the validated allowed origins. Apply the protection as middleware via protection.Handler(next); loopback protection is left enabled by default.
func NewStreamableHTTPOptions(cfg StreamableHTTPConfig) (*sdkmcp.StreamableHTTPOptions, *http.CrossOriginProtection, error) {
	protection := http.NewCrossOriginProtection()
	for _, origin := range cfg.AllowedOrigins {
		normalized, err := security.ValidateAllowedOrigin(origin)
		if err != nil {
			return nil, nil, err
		}
		if err := protection.AddTrustedOrigin(normalized); err != nil {
			return nil, nil, fmt.Errorf("invalid allowed origin %q: %w", origin, err)
		}
	}
	return &sdkmcp.StreamableHTTPOptions{
		Stateless:                  cfg.Stateless,
		JSONResponse:               cfg.JSONResponse,
		Logger:                     cfg.Logger,
		EventStore:                 cfg.EventStore,
		SessionTimeout:             cfg.SessionTimeout,
		DisableLocalhostProtection: cfg.DisableLocalhostProtection,
	}, protection, nil
}

// StreamableHTTPHandler builds a fully hardened HTTP handler for this server: the SDK Streamable HTTP handler wrapped with Origin cross-origin protection and, when authToken is non-empty, bearer-token authentication. The handler is the outermost wrapper, so unauthenticated or cross-origin requests are rejected before reaching the protocol layer.
func (s *Server) StreamableHTTPHandler(cfg StreamableHTTPConfig, authToken string) (http.Handler, error) {
	opts, protection, err := NewStreamableHTTPOptions(cfg)
	if err != nil {
		return nil, err
	}
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return s.sdk }, opts)
	var inner http.Handler = handler
	if authToken != "" {
		inner, err = security.RequireBearerToken(authToken, inner)
		if err != nil {
			return nil, err
		}
	}
	// Origin/loopback cross-origin protection is the outermost guard, so blocked origins are rejected before the bearer challenge and never observe auth behavior.
	return protection.Handler(inner), nil
}
