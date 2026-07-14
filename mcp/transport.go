package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport identifies a canonical MCP transport name. Only stdio and
// Streamable HTTP are exposed; the obsolete standalone SSE transport is not.
type Transport string

const (
	// TransportStdio is the canonical MCP stdio transport name.
	TransportStdio Transport = "stdio"
	// TransportStreamableHTTP is the canonical MCP Streamable HTTP transport name.
	TransportStreamableHTTP Transport = "streamable_http"
)

// ParseTransport validates a canonical MCP transport name, failing closed on
// any unrecognized value.
func ParseTransport(name string) (Transport, error) {
	switch Transport(name) {
	case TransportStdio, TransportStreamableHTTP:
		return Transport(name), nil
	default:
		return "", fmt.Errorf(
			"unsupported MCP transport %q: use %q or %q",
			name, TransportStdio, TransportStreamableHTTP,
		)
	}
}

// ServeStdio runs the server over the MCP stdio transport until ctx is
// canceled or stdin closes. This is the default transport for local,
// single-client MCP integrations (IDEs, agents).
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.Run(ctx, &sdkmcp.StdioTransport{})
}
