package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

// NewServer creates an MCP server that exposes all tools from the given registry.
// Each registered tool.Callable becomes an MCP tool that external clients can discover and call.
//
// Returns an error when registry is nil; otherwise mirrors the behavior of
// SkillToServerAdapter so callers can compose without nil-pointer panics.
func NewServer(name, version string, registry *tool.Registry, opts ...ServerOption) (*sdkmcp.Server, error) {
	if registry == nil {
		return nil, fmt.Errorf("mcp: tool registry is required")
	}
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
	defs := registry.List()
	for i := range defs {
		def := defs[i]
		callable, ok := registry.Get(def.Name)
		if !ok {
			continue
		}
		if !cfg.allows(def.Name) {
			continue
		}
		registerTool(server, registry, callable, cfg)
	}
	for _, entry := range cfg.prompts {
		server.AddPrompt(entry.prompt, entry.handler)
	}
	for _, entry := range cfg.resources {
		server.AddResource(entry.resource, entry.handler)
	}
	for _, entry := range cfg.resourceTemplates {
		server.AddResourceTemplate(entry.template, entry.handler)
	}

	return server, nil
}

// registerTool adds a single kit tool to the MCP server.
func registerTool(server *sdkmcp.Server, registry *tool.Registry, callable tool.Callable, cfg *serverConfig) {
	def := callable.Definition()

	mcpTool := definitionToMCPTool(def)
	if cfg.prefix != "" {
		mcpTool.Name = cfg.prefix + mcpTool.Name
	}

	handler := makeToolHandler(def.Name, mcpTool.Name, registry, cfg)
	server.AddTool(mcpTool, handler)
}

// makeToolHandler creates an MCP ToolHandler that delegates to the kit registry.
func makeToolHandler(toolName, mcpName string, registry *tool.Registry, cfg *serverConfig) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		ctx, span := observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/mcp", "mcp.request",
			observability.WithSpanKind(observability.SpanKindServer),
			observability.WithSpanAttributes(
				observability.StringAttribute("mcp.method", "tools/call"),
				observability.StringAttribute(semconv.GenAIToolName, toolName),
				observability.StringAttribute("mcp.tool_name", mcpName),
			),
		)
		defer span.End()
		event := ToolAuditEvent{ToolName: toolName, MCPName: mcpName}
		defer func() {
			cfg.auditToolCall(ctx, event)
		}()

		callable, ok := registry.Get(toolName)
		if !ok {
			event.Outcome = "not_found"
			event.Error = fmt.Sprintf("tool %q not found in registry", toolName)
			return nil, fmt.Errorf("tool %q not found in registry", toolName)
		}

		// Extract raw arguments
		var input json.RawMessage
		var session any
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			input = req.Params.Arguments
		}
		if req.Params != nil {
			session = req.Session
		}

		decision, err := cfg.authorizeToolCall(ctx, ToolAuthorizationRequest{
			ToolName:  toolName,
			MCPName:   mcpName,
			Arguments: input,
			Session:   session,
		})
		event.Reason = decision.Reason
		if err != nil {
			event.Outcome = "authorization_error"
			event.Error = err.Error()
			return &sdkmcp.CallToolResult{ //nolint:nilerr // intentional MCP error envelope
				IsError: true,
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: "authorization error"},
				},
			}, nil
		}
		if !decision.Allowed {
			event.Outcome = "denied"
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: deniedMessage(decision.Reason)},
				},
			}, nil
		}

		if cfg.maxInputBytes > 0 && len(input) > cfg.maxInputBytes {
			event.Outcome = "input_too_large"
			event.Error = fmt.Sprintf("input size %d exceeds limit %d", len(input), cfg.maxInputBytes)
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: fmt.Sprintf("input too large: exceeds %d bytes", cfg.maxInputBytes)},
				},
			}, nil
		}

		// Validate input
		if input != nil {
			if vr := callable.Validate(input); !vr.Valid {
				msg := firstValidationError(vr.Errors)
				event.Outcome = "validation_error"
				event.Error = msg
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{
						&sdkmcp.TextContent{Text: fmt.Sprintf("validation error: %s", msg)},
					},
				}, nil
			}
		}

		toolCtx := tool.NewContext(ctx)
		if cfg.maxResultBytes > 0 {
			toolCtx.MaxResultSize = cfg.maxResultBytes
		}
		if req.Params != nil {
			toolCtx.Set("mcp.session", req.Session)
		}

		// Dispatch via the registry so authz → sensitivity → human approval
		// run (D10). The MCP-level authorize step above is preserved for
		// transport-level decisions (decider, allowed-tools); the registry
		// adds tool-envelope-driven HITL.
		result, err := registry.Call(toolCtx, toolName, input)
		if err != nil {
			if errors.Is(err, tool.ErrToolDenied) {
				event.Outcome = "denied"
				event.Error = err.Error()
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{
						&sdkmcp.TextContent{Text: deniedMessage(err.Error())},
					},
				}, nil
			}
			event.Outcome = "tool_error"
			event.Error = err.Error()
			// MCP convention: tool execution errors are surfaced to the client
			// as IsError content payloads, not as transport-level errors.
			return &sdkmcp.CallToolResult{ //nolint:nilerr // intentional MCP error envelope
				IsError: true,
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		if result != nil && result.IsError {
			event.Outcome = "tool_error"
			event.Error = result.Text()
		} else {
			if cfg.maxResultBytes > 0 {
				if size := resultSizeBytes(result); size > cfg.maxResultBytes {
					event.Outcome = "result_too_large"
					event.Error = fmt.Sprintf("result size %d exceeds limit %d", size, cfg.maxResultBytes)
					return &sdkmcp.CallToolResult{
						IsError: true,
						Content: []sdkmcp.Content{
							&sdkmcp.TextContent{Text: fmt.Sprintf("result too large: exceeds %d bytes", cfg.maxResultBytes)},
						},
					}, nil
				}
			}
			if vr := validateToolOutput(callable.Definition(), result); !vr.Valid {
				msg := firstValidationError(vr.Errors)
				event.Outcome = "output_validation_error"
				event.Error = msg
				return &sdkmcp.CallToolResult{
					IsError: true,
					Content: []sdkmcp.Content{
						&sdkmcp.TextContent{Text: fmt.Sprintf("output validation error: %s", msg)},
					},
				}, nil
			}
			event.Outcome = "success"
		}
		return resultToMCPResult(result), nil
	}
}

// ServerOption configures server creation.
type ServerOption func(*serverConfig)

type serverConfig struct {
	title             string
	prefix            string
	serverOpts        *sdkmcp.ServerOptions
	allowedTools      map[string]struct{}
	decider           authz.Decider
	auditor           observability.Auditor
	maxInputBytes     int
	maxResultBytes    int
	prompts           []promptEntry
	resources         []resourceEntry
	resourceTemplates []resourceTemplateEntry
}

type promptEntry struct {
	prompt  *sdkmcp.Prompt
	handler sdkmcp.PromptHandler
}

type resourceEntry struct {
	resource *sdkmcp.Resource
	handler  sdkmcp.ResourceHandler
}

type resourceTemplateEntry struct {
	template *sdkmcp.ResourceTemplate
	handler  sdkmcp.ResourceHandler
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

// WithAllowedTools restricts the exposed MCP tool set to the named registry tools.
// When omitted, all registered tools are exposed.
func WithAllowedTools(names ...string) ServerOption {
	return func(c *serverConfig) {
		c.allowedTools = make(map[string]struct{}, len(names))
		for _, name := range names {
			c.allowedTools[name] = struct{}{}
		}
	}
}

// WithAuthzDecider sets the canonical authz decider for MCP tool calls.
func WithAuthzDecider(decider authz.Decider) ServerOption {
	return func(c *serverConfig) { c.decider = decider }
}

// WithAuditor sets the observability auditor for MCP tool invocation outcomes.
func WithAuditor(auditor observability.Auditor) ServerOption {
	return func(c *serverConfig) { c.auditor = auditor }
}

// WithMaxInputBytes rejects MCP tool calls whose JSON arguments exceed this size.
func WithMaxInputBytes(limit int) ServerOption {
	return func(c *serverConfig) { c.maxInputBytes = limit }
}

// WithMaxResultBytes rejects MCP tool results whose serialized output exceeds this size.
func WithMaxResultBytes(limit int) ServerOption {
	return func(c *serverConfig) { c.maxResultBytes = limit }
}

// WithPrompt registers an MCP prompt on the server.
func WithPrompt(prompt *sdkmcp.Prompt, handler sdkmcp.PromptHandler) ServerOption {
	return func(c *serverConfig) {
		c.prompts = append(c.prompts, promptEntry{prompt: prompt, handler: handler})
	}
}

// WithResource registers an MCP resource on the server.
func WithResource(resource *sdkmcp.Resource, handler sdkmcp.ResourceHandler) ServerOption {
	return func(c *serverConfig) {
		c.resources = append(c.resources, resourceEntry{resource: resource, handler: handler})
	}
}

// WithResourceTemplate registers an MCP resource template on the server.
func WithResourceTemplate(template *sdkmcp.ResourceTemplate, handler sdkmcp.ResourceHandler) ServerOption {
	return func(c *serverConfig) {
		c.resourceTemplates = append(c.resourceTemplates, resourceTemplateEntry{template: template, handler: handler})
	}
}

func (c *serverConfig) allows(name string) bool {
	if len(c.allowedTools) == 0 {
		return true
	}
	_, ok := c.allowedTools[name]
	return ok
}

func (c *serverConfig) authorizeToolCall(ctx context.Context, req ToolAuthorizationRequest) (authz.Decision, error) {
	if c.decider == nil {
		return authz.Decision{Allowed: true, Reason: "no_decider"}, nil
	}
	return c.decider.Decide(ctx, authzRequest(req))
}

func (c *serverConfig) auditToolCall(ctx context.Context, event ToolAuditEvent) {
	audit(ctx, c.auditor, event)
}

func deniedMessage(reason string) string {
	if reason == "" {
		return "tool call denied"
	}
	return "tool call denied: " + reason
}

func resultSizeBytes(result *tool.Result) int {
	switch {
	case result == nil:
		return 0
	case len(result.Output) > 0:
		return len(result.Output)
	default:
		return len([]byte(result.Text()))
	}
}

func validateToolOutput(def tool.Definition, result *tool.Result) schema.ValidationResult {
	if def.OutputSchema == nil || result == nil || result.IsError {
		return schema.ValidationResult{Valid: true}
	}
	if len(result.Output) > 0 {
		return schema.Validate(def.OutputSchema, result.Output)
	}
	return schema.Validate(def.OutputSchema, result.Text())
}

// firstValidationError returns the first validation error message, or a
// generic fallback when the slice is empty. Schema validators are expected to
// populate Errors when Valid is false, but the guard avoids out-of-bounds
// access if a backend ever returns Valid=false with an empty Errors slice.
func firstValidationError(errs []schema.ValidationError) string {
	if len(errs) == 0 {
		return "validation failed"
	}
	return errs[0].Error()
}
