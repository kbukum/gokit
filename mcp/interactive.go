package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/handlers"
)

// InteractiveToolHandler handles a tools/call that may request additional input
// from the client (sampling, elicitation, roots) via the multi round-trip
// pattern (SEP-2322). See Server.AddInteractiveTool.
type InteractiveToolHandler = handlers.InteractiveToolHandler

// AddInteractiveTool registers an interactive tool that can drive server-to-client
// sampling, elicitation, and roots requests through the multi round-trip pattern
// (SEP-2322), the mechanism the current MCP protocol version requires for
// server-initiated input.
//
// The handler runs once per round trip: it returns a *sdkmcp.CallToolResult
// carrying an InputRequests map (with an opaque RequestState) to ask the client
// for input, and is invoked again with req.Params.InputResponses populated to
// produce the final result. Read those responses with SampleResponse,
// ElicitResponse, and RootsResponse so untrusted model output and elicited
// content stay size-limited and audited. Every invocation passes the same
// fail-closed call gates as registry tools (allow-list, input size limit,
// authorization, audit).
func (s *Server) AddInteractiveTool(mcpTool *sdkmcp.Tool, handler InteractiveToolHandler) {
	s.handler.AddInteractiveTool(mcpTool, handler)
}

// SampleResponse extracts, size-limits, and audits the sampling response the
// client returned for label on a re-invoked interactive tool call.
// The result is untrusted model output and fails closed when oversized.
func (s *Server) SampleResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.CreateMessageWithToolsResult, error) {
	return s.handler.SampleResponse(ctx, req, label)
}

// ElicitResponse extracts, size-limits, and audits the elicitation response the
// client returned for label on a re-invoked interactive tool call.
// The submitted content is untrusted user input and fails closed when oversized.
func (s *Server) ElicitResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.ElicitResult, error) {
	return s.handler.ElicitResponse(ctx, req, label)
}

// RootsResponse extracts and audits the roots response the client returned for
// label on a re-invoked interactive tool call.
func (s *Server) RootsResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.ListRootsResult, error) {
	return s.handler.RootsResponse(ctx, req, label)
}
