package llm

// Message represents a single chat message.
type Message struct {
	Role    string `json:"role" yaml:"role"` // "system", "user", "assistant"
	Content string `json:"content" yaml:"content"`
}

// CompletionRequest is the universal input for all LLM providers.
type CompletionRequest struct {
	// Model overrides the adapter's default model.
	Model string `json:"model,omitempty" yaml:"model"`
	// Messages is the conversation history.
	Messages []Message `json:"messages" yaml:"messages"`
	// SystemPrompt is prepended as a system message.
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt"`
	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative).
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature"`
	// MaxTokens limits the response length. 0 means provider default.
	MaxTokens int `json:"max_tokens,omitempty" yaml:"max_tokens"`
	// Stream requests streaming mode. Set automatically by Adapter.Stream().
	Stream bool `json:"stream,omitempty" yaml:"stream"`
	// Extra holds provider-specific fields that don't fit the universal schema.
	// Dialects may inspect this for provider-specific features.
	Extra map[string]any `json:"extra,omitempty" yaml:"extra"`
}

// CompletionResponse is the universal output from all LLM providers.
type CompletionResponse struct {
	// Content is the generated text.
	Content string `json:"content"`
	// Model is the model that produced the response.
	Model string `json:"model"`
	// Usage reports token consumption.
	Usage Usage `json:"usage"`
}

// StreamChunk is a single piece of a streamed response.
type StreamChunk struct {
	// Content is the text fragment.
	Content string `json:"content"`
	// Done indicates this is the final chunk.
	Done bool `json:"done"`
	// Err is set when a streaming error occurs.
	Err error `json:"-"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
