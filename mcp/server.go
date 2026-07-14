package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/handlers"
	"github.com/kbukum/gokit/tool"
)

// Server is a hardened, protocol-shaped MCP server backed by a gokit
// tool.Registry. It wraps the official MCP Go SDK server, adding a fail-closed
// hardening chain (capability allow-list, size limits, schema validation,
// authorization, destructive-tool gate, and audit) and typed server-to-client
// helpers for sampling, elicitation, roots, and logging.
//
// Construct a Server with NewServer and drive it over a transport with Run or
// ServeStdio, or by mounting the SDK server behind the Streamable HTTP handler
// built via NewStreamableHTTPOptions.
type Server struct {
	sdk     *sdkmcp.Server
	handler *handlers.Handler
}

// NewServer creates a hardened MCP Server exposing the tools in registry.
//
// It returns an error when registry is nil. Tool, prompt, and resource
// registration, the capability allow-list, size limits, authorization, and
// audit are all applied during construction; nothing is exposed that the
// configuration did not explicitly allow.
func NewServer(name, version string, registry *tool.Registry, opts ...ServerOption) (*Server, error) {
	if registry == nil {
		return nil, fmt.Errorf("mcp: tool registry is required")
	}
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}

	impl := &sdkmcp.Implementation{Name: name, Version: version}
	if cfg.title != "" {
		impl.Title = cfg.title
	}

	sdkServer := sdkmcp.NewServer(impl, cfg.serverOptions())

	policy := cfg.policy
	handler := handlers.New(sdkServer, registry, &policy, cfg.prefix)
	handler.Register(cfg.prompts, cfg.resources, cfg.resourceTemplates)

	return &Server{sdk: sdkServer, handler: handler}, nil
}

// SDK returns the underlying MCP Go SDK server for advanced composition (for
// example, mounting behind a Streamable HTTP handler).
func (s *Server) SDK() *sdkmcp.Server { return s.sdk }

// Run serves the MCP protocol over t until the context is canceled or the
// peer disconnects.
func (s *Server) Run(ctx context.Context, t sdkmcp.Transport) error {
	return s.sdk.Run(ctx, t)
}

// Connect binds the server to a single transport session and returns it. The
// caller owns the returned session lifecycle and must Close it.
func (s *Server) Connect(ctx context.Context, t sdkmcp.Transport, opts *sdkmcp.ServerSessionOptions) (*sdkmcp.ServerSession, error) {
	return s.sdk.Connect(ctx, t, opts)
}
