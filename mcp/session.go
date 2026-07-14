package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sample performs a server-to-client sampling request (sampling/createMessage)
// over the given session, enforcing the configured result size limit and
// auditing the outcome. The returned message is untrusted model output.
func (s *Server) Sample(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.CreateMessageParams) (*sdkmcp.CreateMessageResult, error) {
	return s.handler.Sample(ctx, ss, params)
}

// Elicit performs a server-to-client elicitation request over the given
// session, enforcing the configured result size limit and auditing the
// outcome. The submitted content is untrusted user input.
func (s *Server) Elicit(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ElicitParams) (*sdkmcp.ElicitResult, error) {
	return s.handler.Elicit(ctx, ss, params)
}

// ListRoots asks the connected client for its roots over the given session,
// auditing the outcome.
func (s *Server) ListRoots(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ListRootsParams) (*sdkmcp.ListRootsResult, error) {
	return s.handler.ListRoots(ctx, ss, params)
}

// Log sends a logging/message notification to the connected client over the
// given session.
func (s *Server) Log(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.LoggingMessageParams) error {
	return s.handler.Log(ctx, ss, params)
}

// NotifyProgress sends a progress notification to the connected client over
// the given session.
func (s *Server) NotifyProgress(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ProgressNotificationParams) error {
	return s.handler.NotifyProgress(ctx, ss, params)
}
