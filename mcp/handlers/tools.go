package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/mcp/convert"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/tool"
)

// registerTool adds a single registry tool to the SDK server behind the hardened call handler.
func (h *Handler) registerTool(callable tool.Callable) {
	def := callable.Definition()
	mcpTool := convert.ToMCPTool(def)
	if h.prefix != "" {
		mcpTool.Name = h.prefix + mcpTool.Name
	}
	h.sdk.AddTool(mcpTool, h.makeToolHandler(def.Name, mcpTool.Name))
}

// makeToolHandler builds the fail-closed tools/call handler: allow-list -> input size limit -> schema validation -> authorization -> registry dispatch (which applies the sensitivity/human-approval gate) -> output size limit -> output schema validation -> audit.
func (h *Handler) makeToolHandler(toolName, mcpName string) sdkmcp.ToolHandler {
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

		callable, ok := h.registry.Get(toolName)
		if !ok {
			event.Outcome = security.OutcomeNotFound
			event.Error = fmt.Sprintf("tool %q not found in registry", toolName)
			return errorResult(fmt.Sprintf("tool not found: %s", toolName)), nil
		}

		if input != nil {
			if vr := callable.Validate(input); !vr.Valid {
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

		toolCtx := tool.NewContext(ctx)
		if h.policy.MaxResultBytes > 0 {
			toolCtx.MaxResultSize = h.policy.MaxResultBytes
		}

		result, err := h.registry.Call(toolCtx, toolName, input)
		if err != nil {
			if errors.Is(err, tool.ErrToolDenied) {
				event.Outcome, event.Error = security.OutcomeDenied, err.Error()
				return errorResult(security.DeniedMessage(err.Error())), nil
			}
			event.Outcome, event.Error = security.OutcomeToolError, err.Error()
			// MCP convention: tool execution errors surface as IsError content, not as transport-level errors.
			return errorResult(err.Error()), nil //nolint:nilerr // fail-closed MCP error envelope
		}

		if result == nil {
			event.Outcome, event.Error = security.OutcomeToolError, "tool returned no result"
			return errorResult("tool returned no result"), nil
		}

		if result.IsError {
			event.Outcome, event.Error = security.OutcomeToolError, result.Text()
			return convert.ToMCPResult(result), nil
		}

		if size := security.ResultSizeBytes(result); h.policy.ResultTooLarge(size) {
			event.Outcome = security.OutcomeResultTooLarge
			event.Error = fmt.Sprintf("result size %d exceeds limit %d", size, h.policy.MaxResultBytes)
			return errorResult(fmt.Sprintf("result too large: exceeds %d bytes", h.policy.MaxResultBytes)), nil
		}
		if vr := security.ValidateOutput(callable.Definition(), result); !vr.Valid {
			msg := security.FirstValidationError(vr.Errors)
			event.Outcome, event.Error = security.OutcomeOutputInvalid, msg
			return errorResult(fmt.Sprintf("output validation error: %s", msg)), nil
		}
		event.Outcome = security.OutcomeSuccess
		return convert.ToMCPResult(result), nil
	}
}

// errorResult builds an MCP CallToolResult carrying a single text error, the canonical way to surface a gated or failed call to the client.
func errorResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}
