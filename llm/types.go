package llm

// Message represents a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest holds parameters for an LLM completion call.
type CompletionRequest struct {
	Model        string    `json:"model,omitempty"`
	Messages     []Message `json:"messages"`
	Temperature  float64   `json:"temperature,omitempty"`
	MaxTokens    int       `json:"max_tokens,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
}

// CompletionResponse holds the result of a completion call.
type CompletionResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
}

// StreamChunk represents a single streamed token or partial response.
type StreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Err     error  `json:"-"`
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
