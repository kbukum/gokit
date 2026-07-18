package mcp

import (
	"context"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/handlers"
	"github.com/kbukum/gokit/mcp/security"
)

// serverConfig is the assembled, validated configuration for a Server. It is populated by ServerOption values grouped by concern across the handler files and carries the security.Policy consulted by every gated access.
type serverConfig struct {
	title  string
	prefix string

	// policy holds the fail-closed hardening configuration (allow-lists, authorization, size limits, and audit).
	policy security.Policy

	// protocol features
	prompts           []handlers.PromptEntry
	resources         []handlers.ResourceEntry
	resourceTemplates []handlers.ResourceTemplateEntry

	// capability handlers (wired into ServerOptions by serverOptions)
	logger                  *slog.Logger
	instructions            string
	rootsListChangedHandler func(context.Context, *sdkmcp.RootsListChangedRequest)
	progressHandler         func(context.Context, *sdkmcp.ProgressNotificationServerRequest)
	subscribeHandler        func(context.Context, *sdkmcp.SubscribeRequest) error
	unsubscribeHandler      func(context.Context, *sdkmcp.UnsubscribeRequest) error

	// escape hatch base options supplied by the caller
	baseServerOpts *sdkmcp.ServerOptions
}

// serverOptions assembles the SDK ServerOptions from the configured capability handlers, overlaying them onto any caller-supplied base options. The SDK infers advertised capabilities (tools/prompts/resources) from registered features and from the subscribe/roots handlers wired here.
func (c *serverConfig) serverOptions() *sdkmcp.ServerOptions {
	var opts sdkmcp.ServerOptions
	if c.baseServerOpts != nil {
		opts = *c.baseServerOpts
	}
	if c.instructions != "" {
		opts.Instructions = c.instructions
	}
	if c.logger != nil {
		opts.Logger = c.logger
	}
	if c.rootsListChangedHandler != nil {
		opts.RootsListChangedHandler = c.rootsListChangedHandler
	}
	if c.progressHandler != nil {
		opts.ProgressNotificationHandler = c.progressHandler
	}
	if c.subscribeHandler != nil {
		opts.SubscribeHandler = c.subscribeHandler
	}
	if c.unsubscribeHandler != nil {
		opts.UnsubscribeHandler = c.unsubscribeHandler
	}
	return &opts
}
