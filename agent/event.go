package agent

import (
	"encoding/json"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// AgentEvent is the sealed sum-type streamed by [Agent.Stream]. It surfaces the full
// turn lifecycle — turn boundaries, provider token deltas, tool execution, context
// compaction, and terminal completion — so callers can drive an interactive UI without
// re-running the loop themselves.
//
// The set is closed: only the concrete types declared in this file satisfy it, which lets
// consumers exhaustively type-switch over the stream.
type AgentEvent interface {
	agentEvent()
}

// TurnStart marks the beginning of turn number Turn (1-based).
type TurnStart struct {
	Turn int `json:"turn"`
}

func (TurnStart) agentEvent() {}

// LLMDelta wraps a single provider streaming event (text/reasoning/tool-use deltas, usage,
// or the terminal [llm.MessageComplete]) observed during the current turn's model call.
type LLMDelta struct {
	Event llm.StreamEvent `json:"event"`
}

func (LLMDelta) agentEvent() {}

// ToolExecuting is emitted immediately before a tool call is dispatched.
type ToolExecuting struct {
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

func (ToolExecuting) agentEvent() {}

// ToolComplete reports the outcome of a dispatched tool call. Exactly one of Result or Err
// is meaningful: Err is non-nil when the call failed.
type ToolComplete struct {
	ToolUseID string       `json:"tool_use_id"`
	Name      string       `json:"name"`
	Result    *tool.Result `json:"result,omitempty"`
	Err       error        `json:"-"`
}

func (ToolComplete) agentEvent() {}

// Compacted reports a context-window compaction, with the token counts before and after.
type Compacted struct {
	OldTokens int `json:"old_tokens"`
	NewTokens int `json:"new_tokens"`
}

func (Compacted) agentEvent() {}

// TurnComplete marks the end of a turn, carrying the assistant message and per-turn usage.
type TurnComplete struct {
	Turn    int                   `json:"turn"`
	Message chat.AssistantMessage `json:"message"`
	Usage   llm.Usage             `json:"usage"`
}

func (TurnComplete) agentEvent() {}

// RunComplete is the terminal event on every stream. Result is always populated with the
// run's final state; Err is non-nil when the run stopped on a budget/limit/cancellation error.
type RunComplete struct {
	Result *Result `json:"result,omitempty"`
	Err    error   `json:"-"`
}

func (RunComplete) agentEvent() {}
