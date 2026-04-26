package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/tool"
)

// NewServer creates an MCP server that exposes all tools from the given registry.
// Each registered tool.Callable becomes an MCP tool that external clients can discover and call.
func NewServer(name, version string, registry *tool.Registry, opts ...ServerOption) *sdkmcp.Server {
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}

	impl := &sdkmcp.Implementation{
		Name:    name,
		Version: version,
	}
	if cfg.title != "" {
		impl.Title = cfg.title
	}

	server := sdkmcp.NewServer(impl, cfg.serverOpts)

	// Register each tool from the kit registry
	for _, def := range registry.List() {
		callable, ok := registry.Get(def.Name)
		if !ok {
			continue
		}
		registerTool(server, registry, callable, cfg.prefix)
	}

	return server
}

// registerTool adds a single kit tool to the MCP server.
func registerTool(server *sdkmcp.Server, registry *tool.Registry, callable tool.Callable, prefix string) {
	def := callable.Definition()

	mcpTool := definitionToMCPTool(def)
	if prefix != "" {
		mcpTool.Name = prefix + mcpTool.Name
	}

	handler := makeToolHandler(def.Name, registry)
	server.AddTool(mcpTool, handler)
}

// makeToolHandler creates an MCP ToolHandler that delegates to the kit registry.
func makeToolHandler(toolName string, registry *tool.Registry) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		callable, ok := registry.Get(toolName)
		if !ok {
			return nil, fmt.Errorf("tool %q not found in registry", toolName)
		}

		// Extract raw arguments
		var input json.RawMessage
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			input = req.Params.Arguments
		}

		// Validate input
		if input != nil {
			if vr := callable.Validate(input); !vr.Valid {
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{
						&sdkmcp.TextContent{Text: fmt.Sprintf("validation error: %s", vr.Errors[0].Error())},
					},
				}, nil
			}
		}

		toolCtx := tool.NewContext(ctx)
		if req.Params != nil {
			toolCtx.Set("mcp.session", req.Session)
		}

		result, err := callable.Call(toolCtx, input)
		if err != nil {
			// MCP convention: tool execution errors are surfaced to the client
			// as IsError content payloads, not as transport-level errors.
			return &sdkmcp.CallToolResult{ //nolint:nilerr // intentional MCP error envelope
				IsError: true,
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return resultToMCPResult(result), nil
	}
}

// ServerOption configures server creation.
type ServerOption func(*serverConfig)

type serverConfig struct {
	title      string
	prefix     string
	serverOpts *sdkmcp.ServerOptions
}

// WithTitle sets the server's display title.
func WithTitle(title string) ServerOption {
	return func(c *serverConfig) { c.title = title }
}

// WithPrefix prepends a namespace to all tool names (e.g., "myservice_").
func WithPrefix(prefix string) ServerOption {
	return func(c *serverConfig) { c.prefix = prefix }
}

// WithServerOptions passes options directly to the MCP SDK server.
func WithServerOptions(opts *sdkmcp.ServerOptions) ServerOption {
	return func(c *serverConfig) { c.serverOpts = opts }
}
