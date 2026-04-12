package agent

import (
	"encoding/json"

	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// Domain-specific hook event types for the agent loop.
// These implement the generic hook.Event interface so they can
// be used with hook.Registry, but they live in the agent package
// because they carry agent/llm/tool domain knowledge.

// EventType constants for agent lifecycle hooks.
const (
	EventPreToolCall       hook.EventType = "pre_tool_call"
	EventPostToolCall      hook.EventType = "post_tool_call"
	EventPreLLMCall        hook.EventType = "pre_llm_call"
	EventPostLLMCall       hook.EventType = "post_llm_call"
	EventOnError           hook.EventType = "on_error"
	EventTurnStart         hook.EventType = "turn_start"
	EventTurnEnd           hook.EventType = "turn_end"
	EventContextCompacted  hook.EventType = "context_compacted"
	EventModelSwitched     hook.EventType = "model_switched"
	EventMemoryLoaded      hook.EventType = "memory_loaded"
	EventToolsParallelized hook.EventType = "tools_parallelized"
)

// PreToolCall is emitted before a tool is invoked.
type PreToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (PreToolCall) Type() hook.EventType { return EventPreToolCall }

// PostToolCall is emitted after a tool finishes.
type PostToolCall struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result *tool.Result    `json:"result"`
	Err    error           `json:"-"`
}

func (PostToolCall) Type() hook.EventType { return EventPostToolCall }

// PreLLMCall is emitted before an LLM completion request.
type PreLLMCall struct {
	Request llm.CompletionRequest `json:"request"`
}

func (PreLLMCall) Type() hook.EventType { return EventPreLLMCall }

// PostLLMCall is emitted after an LLM completion response.
type PostLLMCall struct {
	Response llm.CompletionResponse `json:"response"`
	Err      error                  `json:"-"`
}

func (PostLLMCall) Type() hook.EventType { return EventPostLLMCall }

// OnError is emitted when an error occurs during the agent loop.
type OnError struct {
	Err    error  `json:"-"`
	Source string `json:"source"`
}

func (OnError) Type() hook.EventType { return EventOnError }

// TurnStart is emitted at the beginning of an agent turn.
type TurnStart struct {
	Turn int `json:"turn"`
}

func (TurnStart) Type() hook.EventType { return EventTurnStart }

// TurnEnd is emitted at the end of an agent turn.
type TurnEnd struct {
	Turn    int                  `json:"turn"`
	Message llm.AssistantMessage `json:"message"`
}

func (TurnEnd) Type() hook.EventType { return EventTurnEnd }

// ContextCompacted is emitted after the context window is compacted.
type ContextCompacted struct {
	OldTokens int    `json:"old_tokens"`
	NewTokens int    `json:"new_tokens"`
	Strategy  string `json:"strategy"`
}

func (ContextCompacted) Type() hook.EventType { return EventContextCompacted }

// ModelSwitched is emitted when the model override changes during a session.
type ModelSwitched struct {
	PreviousModel string `json:"previous_model"`
	NewModel      string `json:"new_model"`
	Reason        string `json:"reason,omitempty"`
}

func (ModelSwitched) Type() hook.EventType { return EventModelSwitched }

// MemoryLoaded is emitted when conversation history is loaded from memory.
type MemoryLoaded struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
}

func (MemoryLoaded) Type() hook.EventType { return EventMemoryLoaded }

// ToolsParallelized is emitted when tools are executed in parallel.
type ToolsParallelized struct {
	ToolNames []string `json:"tool_names"`
	Count     int      `json:"count"`
}

func (ToolsParallelized) Type() hook.EventType { return EventToolsParallelized }
