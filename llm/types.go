package llm

import "github.com/kbukum/gokit/tool"

// Standard message role constants.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// CompletionRequest is the universal input for all LLM providers.
type CompletionRequest struct {
	// Model overrides the adapter's default model.
	Model string `json:"model,omitempty" yaml:"model"`
	// Messages is the conversation history.
	// Serialization is handled by the Dialect, not json.Marshal.
	Messages []Message `json:"-"`
	// SystemPrompt is prepended as a system message.
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt"`
	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative).
	Temperature *float64 `json:"temperature,omitempty" yaml:"temperature"`
	// MaxTokens limits the response length. 0 means provider default.
	MaxTokens int `json:"max_tokens,omitempty" yaml:"max_tokens"`
	// Stream requests streaming mode. Set automatically by Adapter.Stream().
	Stream bool `json:"stream,omitempty" yaml:"stream"`
	// Tools is the list of tools available to the model.
	Tools []tool.Definition `json:"tools,omitempty" yaml:"tools,omitempty"`
	// ToolChoice controls how the model selects tools.
	ToolChoice *ToolChoice `json:"tool_choice,omitempty" yaml:"tool_choice,omitempty"`
	// Extra holds provider-specific fields that don't fit the universal schema.
	Extra map[string]any `json:"extra,omitempty" yaml:"extra"`
}

// CompletionResponse is the universal output from all LLM providers.
type CompletionResponse struct {
	// Message is the assistant's response.
	Message AssistantMessage `json:"message"`
	// Model is the model that produced the response.
	Model string `json:"model"`
	// Usage reports token consumption.
	Usage Usage `json:"usage"`
	// StopReason indicates why the model stopped generating.
	StopReason StopReason `json:"stop_reason,omitempty"`
}

// Text extracts the text content from the response message.
func (r *CompletionResponse) Text() string {
	return r.Message.Text()
}

// HasToolCalls returns true if the response contains tool call requests.
func (r *CompletionResponse) HasToolCalls() bool {
	return r.Message.HasToolCalls()
}

// StreamChunk is a single piece of a streamed response.
type StreamChunk struct {
	// Content is the text fragment.
	Content string `json:"content"`
	// Done indicates this is the final chunk.
	Done bool `json:"done"`
	// Err is set when a streaming error occurs.
	Err error `json:"-"`
	// ToolCalls contains incremental tool call data during streaming.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- Tool calling types ---

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	// ID uniquely identifies this tool call within the response.
	ID string `json:"id"`
	// Type is the call type (always "function" for now).
	Type string `json:"type"`
	// Function contains the function name and arguments.
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function invocation details.
type FunctionCall struct {
	// Name is the tool function name.
	Name string `json:"name"`
	// Arguments is a JSON string containing the function arguments.
	Arguments string `json:"arguments"`
}

// ToolResult feeds tool execution output back to the LLM as a message.
type ToolResult struct {
	// ToolCallID links back to the ToolCall.ID this result responds to.
	ToolCallID string `json:"tool_call_id"`
	// Content is the tool's output as a string.
	Content string `json:"content"`
	// IsError indicates the tool encountered an error.
	IsError bool `json:"is_error,omitempty"`
}

// ToMessage converts a ToolResult into a ToolResultMessage.
func (r ToolResult) ToMessage() ToolResultMessage {
	return ToolResultMsg(r.ToolCallID, r.Content, r.IsError)
}

// ToolChoice controls how the model selects tools.
type ToolChoice struct {
	// Mode controls tool selection: "auto", "none", "required", or "specific".
	Mode string `json:"mode"`
	// Function specifies which tool to call when Mode is "specific".
	Function string `json:"function,omitempty"`
}

// Predefined tool choice modes.
var (
	ToolChoiceAuto     = &ToolChoice{Mode: "auto"}
	ToolChoiceNone     = &ToolChoice{Mode: "none"}
	ToolChoiceRequired = &ToolChoice{Mode: "required"}
)

// ToolChoiceFunc creates a ToolChoice that forces a specific tool.
func ToolChoiceFunc(name string) *ToolChoice {
	return &ToolChoice{Mode: "specific", Function: name}
}
