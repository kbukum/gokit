package agent

import (
	"encoding/json"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

const (
	EventOnStart        hook.EventType = "on_start"
	EventOnLLMRequest   hook.EventType = "on_llm_request"
	EventOnLLMResponse  hook.EventType = "on_llm_response"
	EventOnToolCall     hook.EventType = "on_tool_call"
	EventOnToolResult   hook.EventType = "on_tool_result"
	EventOnMCPRequest   hook.EventType = "on_mcp_request"
	EventOnMCPResult    hook.EventType = "on_mcp_result"
	EventOnStepComplete hook.EventType = "on_step_complete"
	EventOnError        hook.EventType = "on_error"
	EventOnStop         hook.EventType = "on_stop"

	EventMemoryLoaded     hook.EventType = "memory_loaded"
	EventContextCompacted hook.EventType = "context_compacted"
	EventModelSwitched    hook.EventType = "model_switched"
)

type StartEvent struct {
	Turn int `json:"turn"`
}

func (StartEvent) Type() hook.EventType { return EventOnStart }

type LLMRequestEvent struct {
	Request llm.CompletionRequest `json:"request"`
}

func (LLMRequestEvent) Type() hook.EventType { return EventOnLLMRequest }

type LLMResponseEvent struct {
	Request  llm.CompletionRequest   `json:"request"`
	Response *llm.CompletionResponse `json:"response,omitempty"`
}

func (LLMResponseEvent) Type() hook.EventType { return EventOnLLMResponse }

type ToolCallEvent struct {
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

func (ToolCallEvent) Type() hook.EventType { return EventOnToolCall }

type ToolResultEvent struct {
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Result    *tool.Result    `json:"result,omitempty"`
	Err       error           `json:"-"`
}

func (ToolResultEvent) Type() hook.EventType { return EventOnToolResult }

type MCPRequestEvent struct {
	Server string `json:"server"`
	Method string `json:"method"`
}

func (MCPRequestEvent) Type() hook.EventType { return EventOnMCPRequest }

type MCPResultEvent struct {
	Server string `json:"server"`
	Method string `json:"method"`
	Err    error  `json:"-"`
}

func (MCPResultEvent) Type() hook.EventType { return EventOnMCPResult }

type StreamObservedEvent struct {
	Event llm.StreamEvent `json:"event"`
}

func (StreamObservedEvent) Type() hook.EventType { return EventOnStepComplete }

type StepCompleteEvent struct {
	Turn    int                   `json:"turn"`
	Message chat.AssistantMessage `json:"message"`
	Usage   ai.Usage              `json:"usage"`
}

func (StepCompleteEvent) Type() hook.EventType { return EventOnStepComplete }

type ErrorEvent struct {
	Err    error  `json:"-"`
	Source string `json:"source"`
}

func (ErrorEvent) Type() hook.EventType { return EventOnError }

type StopEvent struct {
	Reason StopReason `json:"reason,omitempty"`
	Err    error      `json:"-"`
}

func (StopEvent) Type() hook.EventType { return EventOnStop }

type ContextCompacted struct {
	OldTokens int    `json:"old_tokens"`
	NewTokens int    `json:"new_tokens"`
	Strategy  string `json:"strategy"`
}

func (ContextCompacted) Type() hook.EventType { return EventContextCompacted }

type ModelSwitched struct {
	PreviousModel string `json:"previous_model"`
	NewModel      string `json:"new_model"`
	Reason        string `json:"reason,omitempty"`
}

func (ModelSwitched) Type() hook.EventType { return EventModelSwitched }

type MemoryLoaded struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
}

func (MemoryLoaded) Type() hook.EventType { return EventMemoryLoaded }
