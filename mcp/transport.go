package mcp

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport identifies a canonical MCP transport name.
type Transport string

const (
	// TransportStdio is the canonical MCP stdio transport name.
	TransportStdio Transport = "stdio"
	// TransportStreamableHTTP is the canonical MCP Streamable HTTP transport name.
	TransportStreamableHTTP Transport = "streamable_http"
)

// ParseTransport validates a canonical MCP transport name.
func ParseTransport(name string) (Transport, error) {
	switch Transport(name) {
	case TransportStdio, TransportStreamableHTTP:
		return Transport(name), nil
	default:
		return "", fmt.Errorf(
			"unsupported MCP transport %q: use %q or %q",
			name,
			TransportStdio,
			TransportStreamableHTTP,
		)
	}
}

// StreamableHTTPConfig configures a hardened MCP Streamable HTTP handler.
type StreamableHTTPConfig struct {
	Stateless                  bool
	JSONResponse               bool
	Logger                     *slog.Logger
	EventStore                 sdkmcp.EventStore
	SessionTimeout             time.Duration
	AllowedOrigins             []string
	DisableLocalhostProtection bool
}

// NewStreamableHTTPOptions builds Streamable HTTP options with loopback protection enabled by default.
func NewStreamableHTTPOptions(cfg StreamableHTTPConfig) (*sdkmcp.StreamableHTTPOptions, error) {
	crossOriginProtection := &http.CrossOriginProtection{}
	for _, origin := range cfg.AllowedOrigins {
		normalizedOrigin, err := validateAllowedOrigin(origin)
		if err != nil {
			return nil, err
		}
		if err := crossOriginProtection.AddTrustedOrigin(normalizedOrigin); err != nil {
			return nil, fmt.Errorf("invalid allowed origin %q: %w", origin, err)
		}
	}
	return &sdkmcp.StreamableHTTPOptions{
		Stateless:                  cfg.Stateless,
		JSONResponse:               cfg.JSONResponse,
		Logger:                     cfg.Logger,
		EventStore:                 cfg.EventStore,
		SessionTimeout:             cfg.SessionTimeout,
		DisableLocalhostProtection: cfg.DisableLocalhostProtection,
		CrossOriginProtection:      crossOriginProtection,
	}, nil
}

func validateAllowedOrigin(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("invalid allowed origin %q: %w", origin, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid allowed origin %q: expected scheme and host", origin)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("origin must not contain a path: %s", origin)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin must not contain query or fragment: %s", origin)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("origin must not contain user info: %s", origin)
	}
	if parsed.Opaque != "" {
		return "", fmt.Errorf("invalid allowed origin %q: expected hierarchical URL", origin)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.ForceQuery = false
	return parsed.String(), nil
}
