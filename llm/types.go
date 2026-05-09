package llm

import (
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

type (
	Model        = ai.Model
	ProviderName = ai.Provider
	Usage        = ai.Usage
)

// FinishReason aliases chat.FinishReason within the llm package.
type FinishReason = chat.FinishReason

type CompletionRequest struct {
	Model         string            `json:"model,omitempty" yaml:"model"`
	Messages      []chat.Message    `json:"-"`
	SystemPrompt  string            `json:"system_prompt,omitempty" yaml:"system_prompt"`
	Temperature   *float64          `json:"temperature,omitempty" yaml:"temperature"`
	TopP          *float64          `json:"top_p,omitempty" yaml:"top_p"`
	MaxTokens     int               `json:"max_tokens,omitempty" yaml:"max_tokens"`
	StopSequences []string          `json:"stop_sequences,omitempty" yaml:"stop_sequences,omitempty"`
	Stream        bool              `json:"stream,omitempty" yaml:"stream"`
	Tools         []ai.ToolSpec     `json:"tools,omitempty" yaml:"tools,omitempty"`
	ToolChoice    *ToolChoice       `json:"tool_choice,omitempty" yaml:"tool_choice,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Extra         map[string]any    `json:"extra,omitempty" yaml:"extra"`
}

type CompletionResponse struct {
	Message    chat.AssistantMessage `json:"message"`
	Model      string                `json:"model"`
	Usage      Usage                 `json:"usage"`
	StopReason chat.FinishReason     `json:"stop_reason,omitempty"`
}

func (r *CompletionResponse) Text() string { return r.Message.Text() }

func (r *CompletionResponse) HasToolCalls() bool { return r.Message.HasToolCalls() }

// streamChunk is an llm-internal streaming accumulation type.
// Provider dialects emit these chunks; the public streaming API exposes
// StreamEvent values assembled from them.
type streamChunk = streamwire.Chunk

// streamToolCall carries tool call deltas during streaming; the final
// canonical ai.ToolUseBlock is built after all deltas arrive.
type streamToolCall = streamwire.ToolCall

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

func (r ToolResult) ToMessage() chat.ToolResultMessage {
	return chat.ToolResultMsg(r.ToolCallID, r.Content, r.IsError)
}

type ToolChoice struct {
	Mode     string `json:"mode"`
	Function string `json:"function,omitempty"`
}

var (
	ToolChoiceAuto     = &ToolChoice{Mode: "auto"}
	ToolChoiceNone     = &ToolChoice{Mode: "none"}
	ToolChoiceRequired = &ToolChoice{Mode: "required"}
)

func ToolChoiceFunc(name string) *ToolChoice {
	return &ToolChoice{Mode: "specific", Function: name}
}
