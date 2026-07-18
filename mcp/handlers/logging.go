package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Log sends a logging/message notification to the connected client over the given session.
func (h *Handler) Log(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.LoggingMessageParams) error {
	if ss == nil {
		return fmt.Errorf("mcp: logging requires an active session")
	}
	return ss.Log(ctx, params)
}

// NotifyProgress sends a progress notification to the connected client over the given session.
func (h *Handler) NotifyProgress(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ProgressNotificationParams) error {
	if ss == nil {
		return fmt.Errorf("mcp: progress notification requires an active session")
	}
	return ss.NotifyProgress(ctx, params)
}
