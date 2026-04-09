package llm

import "context"

// Provider is the high-level interface for LLM providers.
// It extends the basic Dialect+Adapter pattern with capability
// introspection and token counting.
//
// Implementations may wrap an Adapter or implement directly.
type Provider interface {
	// Complete sends a completion request and returns the full response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// Stream sends a completion request and returns a channel of StreamEvents.
	// The channel is closed when the stream ends.
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error)

	// Capabilities returns the provider's feature capabilities.
	Capabilities() Capabilities

	// CountTokens returns an approximate token count for the given messages.
	// This is a rough estimate useful for budget tracking; exact counts vary by model.
	CountTokens(messages []Message) int
}

// Capabilities describes what features a provider supports.
type Capabilities struct {
	// SupportsTools indicates whether the provider supports tool/function calling.
	SupportsTools bool `json:"supports_tools"`
	// SupportsVision indicates whether the provider supports image inputs.
	SupportsVision bool `json:"supports_vision"`
	// SupportsThinking indicates whether the provider supports extended thinking.
	SupportsThinking bool `json:"supports_thinking"`
	// SupportsStreaming indicates whether the provider supports streaming responses.
	SupportsStreaming bool `json:"supports_streaming"`
	// MaxContextTokens is the maximum input context window size.
	MaxContextTokens int `json:"max_context_tokens"`
	// MaxOutputTokens is the maximum output token limit.
	MaxOutputTokens int `json:"max_output_tokens"`
	// ModelID is the model identifier string.
	ModelID string `json:"model_id"`
}

// CountTokensApprox provides a simple character-based token estimate.
// This uses the common heuristic of ~4 characters per token.
// Provider implementations may override with more accurate counting.
func CountTokensApprox(messages []Message) int {
	total := 0
	for _, m := range messages {
		switch msg := m.(type) {
		case UserMessage:
			total += countContentTokens(msg.Content)
		case AssistantMessage:
			total += countContentTokens(msg.Content)
			for _, tc := range msg.ToolCalls {
				total += len(tc.Function.Name)/4 + 1
				total += len(tc.Function.Arguments)/4 + 1
			}
		case SystemMessage:
			total += len(msg.Content)/4 + 1
		case ToolResultMessage:
			total += len(msg.Content)/4 + 1
		}
		total += 4 // overhead per message (role, formatting)
	}
	return total
}

func countContentTokens(blocks []ContentBlock) int {
	tokens := 0
	for _, b := range blocks {
		switch block := b.(type) {
		case TextBlock:
			tokens += len(block.Text)/4 + 1
		case ImageBlock:
			tokens += 256 // approximate cost for image
		case ThinkingBlock:
			tokens += len(block.Text)/4 + 1
		}
	}
	return tokens
}
