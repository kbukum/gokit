package hook

import (
	"encoding/json"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// EventType identifies the kind of hook event.
type EventType string

const (
	EventPreToolCall  EventType = "pre_tool_call"
	EventPostToolCall EventType = "post_tool_call"
	EventPreLLMCall   EventType = "pre_llm_call"
	EventPostLLMCall  EventType = "post_llm_call"
	EventOnError      EventType = "on_error"
	EventTurnStart    EventType = "turn_start"
	EventTurnEnd      EventType = "turn_end"
)

// Event is the interface for all hook events.
type Event interface {
	// Type returns the event type identifier.
	Type() EventType
}

// PreToolCall is emitted before a tool is executed.
type PreToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (PreToolCall) Type() EventType { return EventPreToolCall }

// PostToolCall is emitted after a tool execution completes.
type PostToolCall struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result *tool.Result    `json:"result"`
	Err    error           `json:"-"`
}

func (PostToolCall) Type() EventType { return EventPostToolCall }

// PreLLMCall is emitted before an LLM completion request.
type PreLLMCall struct {
	Request llm.CompletionRequest `json:"request"`
}

func (PreLLMCall) Type() EventType { return EventPreLLMCall }

// PostLLMCall is emitted after an LLM completion response.
type PostLLMCall struct {
	Response llm.CompletionResponse `json:"response"`
	Err      error                  `json:"-"`
}

func (PostLLMCall) Type() EventType { return EventPostLLMCall }

// OnError is emitted when an error occurs.
type OnError struct {
	Err    error  `json:"-"`
	Source string `json:"source"`
}

func (OnError) Type() EventType { return EventOnError }

// TurnStart is emitted at the beginning of an agent turn.
type TurnStart struct {
	Turn int `json:"turn"`
}

func (TurnStart) Type() EventType { return EventTurnStart }

// TurnEnd is emitted at the end of an agent turn.
type TurnEnd struct {
	Turn    int                  `json:"turn"`
	Message llm.AssistantMessage `json:"message"`
}

func (TurnEnd) Type() EventType { return EventTurnEnd }

// --- Hook Result ---

// Action determines how the agent loop proceeds after a hook.
type Action int

const (
	// ActionContinue lets execution proceed normally.
	ActionContinue Action = iota
	// ActionAbort stops execution with an optional reason.
	ActionAbort
	// ActionModify lets the handler modify data before proceeding.
	ActionModify
)

// Result is returned by hook handlers to control execution flow.
type Result struct {
	// Action determines whether to continue, abort, or modify.
	Action Action
	// ModifiedData carries replacement data when Action is Modify.
	ModifiedData any
	// Reason explains why execution was aborted (Action == Abort).
	Reason string
}

// Continue returns a Result that lets execution proceed.
func Continue() Result {
	return Result{Action: ActionContinue}
}

// Abort returns a Result that stops execution.
func Abort(reason string) Result {
	return Result{Action: ActionAbort, Reason: reason}
}

// Modify returns a Result that replaces event data.
func Modify(data any) Result {
	return Result{Action: ActionModify, ModifiedData: data}
}

// Handler processes a hook event and returns a result.
type Handler func(Event) Result
