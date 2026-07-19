package agent

import (
	"errors"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// StopReason indicates why the agent loop terminated. It is a type alias for chat.FinishReason
// so LLM-sourced stops require no conversion — the LLM's finish reason IS the stop reason.
// Agent-policy stops (max turns, wall clock, etc.) extend the value set with strings that have no chat equivalent.
type StopReason = chat.FinishReason

const (
	StopEndTurn   StopReason = chat.FinishReasonStop
	StopMaxTokens StopReason = chat.FinishReasonLength
	StopCancelled StopReason = chat.FinishReasonCancelled
	StopError     StopReason = chat.FinishReasonError

	StopMaxTurns     StopReason = "max_turns"
	StopMaxToolCalls StopReason = "max_tool_calls"
	StopWallClock    StopReason = "wall_clock"
	StopCommand      StopReason = "command"
)

var (
	ErrCancelled            = errors.New("agent: canceled")
	ErrWallClockExceeded    = ai.BudgetExceededError{Reason: ai.BudgetExceededWallClock}
	ErrMaxToolCallsExceeded = ai.BudgetExceededError{Reason: ai.BudgetExceededCalls}
	ErrMaxTokensExceeded    = ai.BudgetExceededError{Reason: ai.BudgetExceededTokens}
	ErrMaxTurnsExceeded     = errors.New("agent: max turns exceeded")
)

// Result is the final outcome of an agent run.
type Result struct {
	Messages     []chat.Message        `json:"messages"`
	FinalMessage chat.AssistantMessage `json:"final_message"`
	TotalUsage   llm.Usage             `json:"total_usage"`
	TurnCount    int                   `json:"turn_count"`
	StopReason   StopReason            `json:"stop_reason"`
}
