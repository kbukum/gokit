package mcp

import (
	"context"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/mcp/handlers"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/observability"
)

// ServerOption configures Server creation. Options are applied in order onto a
// serverConfig; granular hardening options write into the embedded
// security.Policy, so a later WithPolicy replaces the whole policy while later
// granular options override individual fields.
type ServerOption func(*serverConfig)

// WithPolicy sets the complete fail-closed hardening policy (allow-lists,
// authorization decider, auditor, and size limits) in one call. Granular
// options such as WithAllowedTools or WithMaxInputBytes override individual
// fields when applied after it.
func WithPolicy(policy security.Policy) ServerOption {
	return func(c *serverConfig) { c.policy = policy }
}

// WithTitle sets the server's display title advertised at initialize.
func WithTitle(title string) ServerOption {
	return func(c *serverConfig) { c.title = title }
}

// WithPrefix prepends a namespace to all exposed tool names (e.g. "myservice_").
func WithPrefix(prefix string) ServerOption {
	return func(c *serverConfig) { c.prefix = prefix }
}

// WithInstructions sets free-form instructions advertised to clients.
func WithInstructions(instructions string) ServerOption {
	return func(c *serverConfig) { c.instructions = instructions }
}

// WithLogger injects the slog.Logger used by the SDK for server activity and
// for server-to-client logging notifications.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(c *serverConfig) { c.logger = logger }
}

// WithServerOptions supplies base SDK server options. Capability handlers
// configured through other options are overlaid onto a copy of these.
func WithServerOptions(opts *sdkmcp.ServerOptions) ServerOption {
	return func(c *serverConfig) { c.baseServerOpts = opts }
}

// WithAllowedTools restricts the exposed MCP tool set to the named registry
// tools. When omitted, all registered tools are exposed. The allow-list is
// enforced both at registration and at call time (fail closed).
func WithAllowedTools(names ...string) ServerOption {
	return func(c *serverConfig) { c.policy.AllowedTools = security.ToSet(names) }
}

// WithAuthzDecider sets the canonical authz decider consulted for every
// tools/call before execution.
func WithAuthzDecider(decider authz.Decider) ServerOption {
	return func(c *serverConfig) { c.policy.Decider = decider }
}

// WithAuditor sets the observability auditor that records every gated tool,
// prompt, and resource access outcome.
func WithAuditor(auditor observability.Auditor) ServerOption {
	return func(c *serverConfig) { c.policy.Auditor = auditor }
}

// WithMaxInputBytes rejects MCP tool calls whose JSON arguments exceed limit.
func WithMaxInputBytes(limit int) ServerOption {
	return func(c *serverConfig) { c.policy.MaxInputBytes = limit }
}

// WithMaxResultBytes rejects MCP tool results whose serialized output exceeds
// limit.
func WithMaxResultBytes(limit int) ServerOption {
	return func(c *serverConfig) { c.policy.MaxResultBytes = limit }
}

// WithPrompt registers a static MCP prompt served by handler. Access is gated
// by the prompt allow-list and recorded through the auditor.
func WithPrompt(prompt *sdkmcp.Prompt, handler sdkmcp.PromptHandler) ServerOption {
	return func(c *serverConfig) {
		c.prompts = append(c.prompts, handlers.PromptEntry{Prompt: prompt, Handler: handler})
	}
}

// WithAllowedPrompts restricts the callable prompt set to the named prompts.
// When omitted, all registered prompts are exposed.
func WithAllowedPrompts(names ...string) ServerOption {
	return func(c *serverConfig) { c.policy.AllowedPrompts = security.ToSet(names) }
}

// WithResource registers a static MCP resource served by handler. Reads are
// gated by the resource allow-list and recorded through the auditor.
func WithResource(resource *sdkmcp.Resource, handler sdkmcp.ResourceHandler) ServerOption {
	return func(c *serverConfig) {
		c.resources = append(c.resources, handlers.ResourceEntry{Resource: resource, Handler: handler})
	}
}

// WithResourceTemplate registers a static MCP resource template served by
// handler. The SDK owns URI-template matching; access is gated by the resource
// allow-list keyed on the template URI.
func WithResourceTemplate(template *sdkmcp.ResourceTemplate, handler sdkmcp.ResourceHandler) ServerOption {
	return func(c *serverConfig) {
		c.resourceTemplates = append(c.resourceTemplates, handlers.ResourceTemplateEntry{Template: template, Handler: handler})
	}
}

// WithAllowedResources restricts readable resources to the named URIs (or
// template URIs). When omitted, all registered resources are exposed.
func WithAllowedResources(uris ...string) ServerOption {
	return func(c *serverConfig) { c.policy.AllowedResources = security.ToSet(uris) }
}

// WithSubscribeHandler wires resource subscribe/unsubscribe handling. Setting
// it advertises the resources.subscribe capability.
func WithSubscribeHandler(
	subscribe func(context.Context, *sdkmcp.SubscribeRequest) error,
	unsubscribe func(context.Context, *sdkmcp.UnsubscribeRequest) error,
) ServerOption {
	return func(c *serverConfig) {
		c.subscribeHandler = subscribe
		c.unsubscribeHandler = unsubscribe
	}
}

// WithRootsListChangedHandler wires a handler invoked when a connected client
// notifies that its roots list changed.
func WithRootsListChangedHandler(h func(context.Context, *sdkmcp.RootsListChangedRequest)) ServerOption {
	return func(c *serverConfig) { c.rootsListChangedHandler = h }
}

// WithProgressHandler wires a handler invoked when a client reports progress
// against a long-running request.
func WithProgressHandler(h func(context.Context, *sdkmcp.ProgressNotificationServerRequest)) ServerOption {
	return func(c *serverConfig) { c.progressHandler = h }
}
