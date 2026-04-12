package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

// Connect establishes a connection to a remote MCP server and returns the session
// along with all discovered tools as kit Callables.
func Connect(ctx context.Context, transport sdkmcp.Transport, opts *ConnectOptions) (*sdkmcp.ClientSession, []tool.Callable, error) {
	if opts == nil {
		opts = &ConnectOptions{}
	}

	impl := &sdkmcp.Implementation{
		Name:    opts.clientName(),
		Version: opts.clientVersion(),
	}
	client := sdkmcp.NewClient(impl, opts.ClientOptions)

	session, err := client.Connect(ctx, transport, opts.SessionOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("mcp connect: %w", err)
	}

	// Discover all tools from the remote server
	var callables []tool.Callable
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			_ = session.Close()
			return nil, nil, fmt.Errorf("mcp list tools: %w", err)
		}
		name := t.Name
		if opts.Prefix != "" {
			name = opts.Prefix + name
		}
		callables = append(callables, &remoteCallable{
			session: session,
			def:     mcpToolToDefinition(t),
			mcpName: t.Name,
			name:    name,
		})
	}

	return session, callables, nil
}

// ConnectOptions configures the MCP client connection.
type ConnectOptions struct {
	// ClientName identifies this client. Defaults to "gokit-mcp-client".
	ClientName string
	// ClientVersion is the client version. Defaults to "1.0.0".
	ClientVersion string
	// Prefix is prepended to all remote tool names for namespacing.
	Prefix string
	// ClientOptions are passed to the MCP SDK client.
	ClientOptions *sdkmcp.ClientOptions
	// SessionOptions are passed to the MCP SDK session.
	SessionOptions *sdkmcp.ClientSessionOptions
}

func (o *ConnectOptions) clientName() string {
	if o.ClientName != "" {
		return o.ClientName
	}
	return "gokit-mcp-client"
}

func (o *ConnectOptions) clientVersion() string {
	if o.ClientVersion != "" {
		return o.ClientVersion
	}
	return "1.0.0"
}

// remoteCallable wraps a remote MCP tool as a kit tool.Callable.
type remoteCallable struct {
	session *sdkmcp.ClientSession
	def     tool.Definition
	mcpName string // original MCP tool name
	name    string // possibly prefixed name
}

func (r *remoteCallable) Definition() tool.Definition {
	def := r.def
	def.Name = r.name
	return def
}

func (r *remoteCallable) Validate(input json.RawMessage) schema.ValidationResult {
	if r.def.InputSchema != nil {
		return schema.Validate(r.def.InputSchema, input)
	}
	return schema.ValidationResult{Valid: true}
}

func (r *remoteCallable) Call(ctx *tool.Context, input json.RawMessage) (*tool.Result, error) {
	// Build arguments from raw JSON
	var args map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("mcp call %q: unmarshal args: %w", r.name, err)
		}
	}

	params := &sdkmcp.CallToolParams{
		Name:      r.mcpName,
		Arguments: args,
	}

	mcpResult, err := r.session.CallTool(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("mcp call %q: %w", r.name, err)
	}

	return mcpResultToResult(mcpResult), nil
}
