package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/schema"
)

// InteractiveToolHandler handles a tools/call that may request additional input
// from the client (sampling, elicitation, roots) via the multi round-trip
// pattern (SEP-2322). The SDK invokes it once to collect the InputRequests it
// returns and again, with req.Params.InputResponses populated, to produce the
// final result.
//
// Handlers read hardened responses with Handler.SampleResponse,
// Handler.ElicitResponse, and Handler.RootsResponse rather than reaching into
// req.Params.InputResponses directly, so untrusted model output and elicited
// content stay size-limited and audited.
type InteractiveToolHandler func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error)

// AddInteractiveTool registers an interactive (multi round-trip) tool behind the
// hardened call gates. The passed mcpTool.Name is the logical tool name used for
// the allow-list and audit; the exposed name has the configured prefix applied.
func (h *Handler) AddInteractiveTool(mcpTool *sdkmcp.Tool, fn InteractiveToolHandler) {
	toolName := mcpTool.Name
	mcpName := mcpTool.Name
	if h.prefix != "" {
		mcpName = h.prefix + mcpTool.Name
		mcpTool.Name = mcpName
	}
	if mcpTool.InputSchema == nil {
		mcpTool.InputSchema = map[string]any{"type": "object"}
	}
	var inputSchema schema.JSON
	if s, ok := mcpTool.InputSchema.(schema.JSON); ok {
		inputSchema = s
	}
	h.sdk.AddTool(mcpTool, h.interactiveToolHandler(toolName, mcpName, inputSchema, fn))
}

// interactiveToolHandler wraps fn with the fail-closed call gates shared with
// makeToolHandler that apply to server-driven interactive tools:
// allow-list -> input size limit -> schema validation -> authorization ->
// dispatch -> result size limit -> audit. The handler owns its own output
// shape because an interactive tool is not backed by a registry callable.
func (h *Handler) interactiveToolHandler(toolName, mcpName string, inputSchema schema.JSON, fn InteractiveToolHandler) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		ctx, span := observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/mcp", "mcp.request",
			observability.WithSpanKind(observability.SpanKindServer),
			observability.WithSpanAttributes(
				observability.StringAttribute(semconv.GenAIOperationName, semconv.OpMCPRequest),
				observability.StringAttribute("mcp.method", "tools/call"),
				observability.StringAttribute(semconv.GenAIToolName, toolName),
				observability.StringAttribute("mcp.tool_name", mcpName),
			),
		)
		defer span.End()

		event := security.ToolAuditEvent{ToolName: toolName, MCPName: mcpName}
		defer func() { h.policy.AuditToolCall(ctx, event) }()

		if !h.policy.AllowsTool(toolName) {
			event.Outcome, event.Reason = security.OutcomeDenied, "not in allow-list"
			return errorResult("tool is not allowed"), nil
		}

		var input json.RawMessage
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			input = req.Params.Arguments
		}
		if h.policy.InputTooLarge(input) {
			event.Outcome = security.OutcomeInputTooLarge
			event.Error = fmt.Sprintf("input size %d exceeds limit %d", len(input), h.policy.MaxInputBytes)
			return errorResult(fmt.Sprintf("input too large: exceeds %d bytes", h.policy.MaxInputBytes)), nil
		}

		// A re-invocation carrying InputResponses skips input validation: the
		// original arguments were already validated on the first round trip and
		// the SDK echoes them back verbatim.
		if input != nil && inputSchema != nil && (req.Params == nil || len(req.Params.InputResponses) == 0) {
			if vr := schema.Validate(inputSchema, input); !vr.Valid {
				msg := security.FirstValidationError(vr.Errors)
				event.Outcome, event.Error = security.OutcomeInvalidInput, msg
				return errorResult(fmt.Sprintf("validation error: %s", msg)), nil
			}
		}

		decision, err := h.policy.Authorize(ctx, security.ToolAuthorizationRequest{
			ToolName:  toolName,
			MCPName:   mcpName,
			Arguments: input,
		})
		event.Reason = decision.Reason
		if err != nil {
			event.Outcome, event.Error = security.OutcomeAuthorizationError, err.Error()
			return errorResult("authorization error"), nil //nolint:nilerr // fail-closed MCP error envelope
		}
		if !decision.Allowed {
			event.Outcome = security.OutcomeDenied
			return errorResult(security.DeniedMessage(decision.Reason)), nil
		}

		result, err := fn(ctx, req)
		if err != nil {
			event.Outcome, event.Error = security.OutcomeToolError, err.Error()
			return errorResult(err.Error()), nil //nolint:nilerr // fail-closed MCP error envelope
		}
		if result == nil {
			event.Outcome, event.Error = security.OutcomeToolError, "interactive tool returned no result"
			return errorResult("tool returned no result"), nil
		}

		// A non-nil InputRequests map (even when empty, which the SDK treats as
		// load-shedding) signals another round trip rather than a final result.
		if result.InputRequests != nil {
			event.Outcome, event.Reason = security.OutcomeInputRequired, "input_required"
			return result, nil
		}
		if size := resultSizeBytes(result); h.policy.ResultTooLarge(size) {
			event.Outcome = security.OutcomeResultTooLarge
			event.Error = fmt.Sprintf("result size %d exceeds limit %d", size, h.policy.MaxResultBytes)
			return errorResult(fmt.Sprintf("result too large: exceeds %d bytes", h.policy.MaxResultBytes)), nil
		}
		if result.IsError {
			event.Outcome, event.Error = security.OutcomeToolError, interactiveErrorText(result)
			return result, nil
		}
		event.Outcome = security.OutcomeSuccess
		return result, nil
	}
}

// SampleResponse extracts, size-limits, and audits the sampling response the
// client returned for label on a re-invoked interactive tool call. The result
// is untrusted model output: an oversized response fails closed rather than
// being handed back to the caller.
func (h *Handler) SampleResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.CreateMessageWithToolsResult, error) {
	res, err := inputResponse[*sdkmcp.CreateMessageWithToolsResult](req, label)
	if err != nil {
		h.auditAccess(ctx, security.AccessKindSampling, "sampling/createMessage", security.OutcomeToolError, err.Error())
		return nil, err
	}
	if reason, tooLarge := h.contentTooLarge(res.Content); tooLarge {
		h.auditAccess(ctx, security.AccessKindSampling, "sampling/createMessage", security.OutcomeResultTooLarge, "sampled content "+reason)
		return nil, fmt.Errorf("mcp: sampled content too large: %s", reason)
	}
	h.auditAccess(ctx, security.AccessKindSampling, "sampling/createMessage", security.OutcomeSuccess, "")
	return res, nil
}

// ElicitResponse extracts, size-limits, and audits the elicitation response the
// client returned for label on a re-invoked interactive tool call. The
// submitted content is untrusted user input and fails closed when oversized.
func (h *Handler) ElicitResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.ElicitResult, error) {
	res, err := inputResponse[*sdkmcp.ElicitResult](req, label)
	if err != nil {
		h.auditAccess(ctx, security.AccessKindElicitation, "elicitation/create", security.OutcomeToolError, err.Error())
		return nil, err
	}
	if reason, tooLarge := h.contentTooLarge(res.Content); tooLarge {
		h.auditAccess(ctx, security.AccessKindElicitation, "elicitation/create", security.OutcomeResultTooLarge, "elicited content "+reason)
		return nil, fmt.Errorf("mcp: elicited content too large: %s", reason)
	}
	h.auditAccess(ctx, security.AccessKindElicitation, "elicitation/create", security.OutcomeSuccess, res.Action)
	return res, nil
}

// RootsResponse extracts and audits the roots response the client returned for
// label on a re-invoked interactive tool call.
func (h *Handler) RootsResponse(ctx context.Context, req *sdkmcp.CallToolRequest, label string) (*sdkmcp.ListRootsResult, error) {
	res, err := inputResponse[*sdkmcp.ListRootsResult](req, label)
	if err != nil {
		h.auditAccess(ctx, security.AccessKindRoots, "roots/list", security.OutcomeToolError, err.Error())
		return nil, err
	}
	h.auditAccess(ctx, security.AccessKindRoots, "roots/list", security.OutcomeSuccess, "")
	return res, nil
}

// inputResponse retrieves the input response for label and asserts its concrete
// type, failing closed when the client omitted it or returned an unexpected shape.
func inputResponse[T sdkmcp.InputResponse](req *sdkmcp.CallToolRequest, label string) (T, error) {
	var zero T
	if req == nil || req.Params == nil || req.Params.InputResponses == nil {
		return zero, fmt.Errorf("mcp: no input response for %q", label)
	}
	resp, ok := req.Params.InputResponses[label]
	if !ok || resp == nil {
		return zero, fmt.Errorf("mcp: no input response for %q", label)
	}
	typed, ok := resp.(T)
	if !ok {
		return zero, fmt.Errorf("mcp: input response for %q has unexpected type %T", label, resp)
	}
	// Reject a typed-nil pointer (it satisfies the assertion but would panic on
	// dereference), keeping the extraction fail-closed.
	if v := reflect.ValueOf(typed); v.Kind() == reflect.Ptr && v.IsNil() {
		return zero, fmt.Errorf("mcp: input response for %q is nil", label)
	}
	return typed, nil
}

func (h *Handler) auditAccess(ctx context.Context, kind, target, outcome, reason string) {
	h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: kind, Target: target, Outcome: outcome, Reason: reason})
}

// resultSizeBytes measures the serialized size of an interactive tool result's
// content, mirroring the tool-result size gate applied to registry tools.
// A marshal failure counts as oversized so unmeasurable content fails closed.
func resultSizeBytes(result *sdkmcp.CallToolResult) int {
	size := 0
	if len(result.Content) > 0 {
		data, err := json.Marshal(result.Content)
		if err != nil {
			return int(^uint(0) >> 1)
		}
		size += len(data)
	}
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return int(^uint(0) >> 1)
		}
		size += len(data)
	}
	return size
}

// interactiveErrorText concatenates the text content of an interactive tool
// error result to surface its message in the audit record. The output is
// bounded so an oversized error body cannot bloat the audit event; the size
// gate that runs before this already caps well-behaved results.
func interactiveErrorText(result *sdkmcp.CallToolResult) string {
	const maxAuditTextBytes = 1024
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			if b.Len()+len(tc.Text) > maxAuditTextBytes {
				b.WriteString(tc.Text[:maxAuditTextBytes-b.Len()])
				break
			}
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
