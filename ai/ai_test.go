package ai_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/semconv"
)

func TestContentPartVariants(t *testing.T) {
	t.Parallel()
	parts := []ai.ContentPart{
		ai.Text{Text: "hello"},
		ai.Image{Source: "memory", MimeType: "image/png", Data: "abc"},
		ai.Audio{Source: "memory", MimeType: "audio/wav"},
		ai.Video{Source: "memory", MimeType: "video/mp4"},
		ai.File{Source: "file:///x", Name: "x.txt"},
		ai.ToolUseBlock{ID: "call_1", Name: "search", Input: map[string]any{"q": "go"}},
		ai.ToolResultBlock{ID: "call_1", Content: "ok"},
	}
	want := []string{"text", "image", "audio", "video", "file", "tool_use", "tool_result"}
	for i, part := range parts {
		if got := part.PartType(); got != want[i] {
			t.Fatalf("part %d type = %q, want %q", i, got, want[i])
		}
	}
}

func TestUsageCostBudgetAndModel(t *testing.T) {
	t.Parallel()
	usage := ai.Usage{InputTokens: 10, OutputTokens: 7, CachedTokens: 3, ReasoningTokens: 2}
	if usage.TotalTokens() != 17 {
		t.Fatalf("total tokens = %d, want 17", usage.TotalTokens())
	}
	budget := ai.Budget{
		MaxTokens: 100,
		MaxCalls:  5,
		MaxCost: ai.Cost{
			Input:    ai.Decimal{Units: 1, Nanos: 25},
			Currency: "USD",
		},
		WallClock: time.Minute,
	}
	if budget.MaxCost.Input.Units != 1 || budget.WallClock != time.Minute {
		t.Fatalf("budget not preserved: %#v", budget)
	}
	model := ai.Model{Name: "gpt", Provider: ai.ProviderOpenAI, Capabilities: ai.Capabilities{Streaming: true, ToolUse: true, MaxInputTokens: 128}}
	if model.Provider != ai.ProviderOpenAI || !model.Capabilities.Streaming || !model.Capabilities.ToolUse {
		t.Fatalf("model capabilities not preserved: %#v", model)
	}
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()
	sentinels := []error{
		ai.ErrRateLimited,
		ai.ErrContextLengthExceeded,
		ai.ErrContentFilter,
		ai.ErrModelOverloaded,
		ai.ErrBudgetExceeded,
		ai.ErrModelNotFound,
		ai.ErrInvalidRequest,
	}
	for _, sentinel := range sentinels {
		if !errors.Is(sentinel, sentinel) {
			t.Fatalf("sentinel did not match itself: %v", sentinel)
		}
	}
	err := ai.BudgetExceededError{Reason: ai.BudgetExceededTokens}
	if !errors.Is(err, ai.ErrBudgetExceeded) || err.Error() != "ai: budget exceeded: tokens" {
		t.Fatalf("budget error mismatch: %v", err)
	}
}

func TestSemconvKeys(t *testing.T) {
	t.Parallel()
	keys := map[string]string{
		"system":                 semconv.GenAISystem,
		"operation_name":         semconv.GenAIOperationName,
		"request_id":             semconv.GenAIRequestID,
		"request_model":          semconv.GenAIRequestModel,
		"request_model_version":  semconv.GenAIRequestModelVersion,
		"request_max_tokens":     semconv.GenAIRequestMaxTokens,
		"request_temperature":    semconv.GenAIRequestTemperature,
		"response_model":         semconv.GenAIResponseModel,
		"response_finish_reason": semconv.GenAIResponseFinishReason,
		"tool_name":              semconv.GenAIToolName,
		"usage_input_tokens":     semconv.GenAIUsageInputTokens,
		"usage_output_tokens":    semconv.GenAIUsageOutputTokens,
		"usage_cached_tokens":    semconv.GenAIUsageCachedTokens,
		"usage_reasoning_tokens": semconv.GenAIUsageReasoningTokens,
	}
	want := map[string]string{
		"system":                 "gen_ai.system",
		"operation_name":         "gen_ai.operation.name",
		"request_id":             "gen_ai.request.id",
		"request_model":          "gen_ai.request.model",
		"request_model_version":  "gen_ai.request.model.version",
		"request_max_tokens":     "gen_ai.request.max_tokens",
		"request_temperature":    "gen_ai.request.temperature",
		"response_model":         "gen_ai.response.model",
		"response_finish_reason": "gen_ai.response.finish_reason",
		"tool_name":              "gen_ai.tool.name",
		"usage_input_tokens":     "gen_ai.usage.input_tokens",
		"usage_output_tokens":    "gen_ai.usage.output_tokens",
		"usage_cached_tokens":    "gen_ai.usage.cached_tokens",
		"usage_reasoning_tokens": "gen_ai.usage.reasoning_tokens",
	}
	for name, got := range keys {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestSemconvOperations(t *testing.T) {
	t.Parallel()
	ops := map[string]string{
		"chat":              semconv.OpChat,
		"text_completion":   semconv.OpTextCompletion,
		"embedding":         semconv.OpEmbedding,
		"agent_turn":        semconv.OpAgentTurn,
		"llm_call":          semconv.OpLLMCall,
		"tool_call":         semconv.OpToolCall,
		"mcp_request":       semconv.OpMCPRequest,
		"stream":            semconv.OpStream,
		"inference_request": semconv.OpInferenceRequest,
	}
	want := map[string]string{
		"chat":              "chat",
		"text_completion":   "text_completion",
		"embedding":         "embedding",
		"agent_turn":        "agent.turn",
		"llm_call":          "llm.call",
		"tool_call":         "tool.call",
		"mcp_request":       "mcp.request",
		"stream":            "stream",
		"inference_request": "inference.request",
	}
	for name, got := range ops {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}
