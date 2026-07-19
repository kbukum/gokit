package agent

import (
	"context"
	"errors"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// runState is the evolving progress of a Run loop: accumulated messages, token usage, and completed turn count.
type runState struct {
	msgs  []chat.Message
	usage llm.Usage
	turns int
}

type budgetState struct {
	usage     llm.Usage
	turn      int
	toolCalls int
}

func (a *Agent) buildResult(st runState, finalMsg chat.AssistantMessage, reason StopReason) *Result {
	return &Result{Messages: st.msgs, FinalMessage: finalMsg, TotalUsage: st.usage, TurnCount: st.turns, StopReason: reason}
}

func (a *Agent) budgetError(ctx context.Context, state budgetState) error {
	if err := ctx.Err(); err != nil {
		return mapContextErr(ctx)
	}
	if a.config.MaxToolCalls > 0 && state.toolCalls > a.config.MaxToolCalls {
		return ErrMaxToolCallsExceeded
	}
	if a.config.MaxTokens > 0 && totalTokens(state.usage) >= a.config.MaxTokens {
		return ErrMaxTokensExceeded
	}
	if state.turn > a.config.MaxTurns {
		return ErrMaxTurnsExceeded
	}
	return nil
}

func mapContextErr(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrWallClockExceeded
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ErrCancelled
	}
	return ctx.Err()
}

func (a *Agent) resultForError(st runState, err error) *Result {
	var final chat.AssistantMessage
	for i := len(st.msgs) - 1; i >= 0; i-- {
		if am, ok := st.msgs[i].(chat.AssistantMessage); ok {
			final = am
			break
		}
	}
	reason := StopError
	switch {
	case errors.Is(err, ErrCancelled):
		reason = StopCancelled
	case errors.Is(err, ErrWallClockExceeded):
		reason = StopWallClock
	case errors.Is(err, ErrMaxToolCallsExceeded):
		reason = StopMaxToolCalls
	case errors.Is(err, ErrMaxTokensExceeded):
		reason = StopMaxTokens
	case errors.Is(err, ErrMaxTurnsExceeded):
		reason = StopMaxTurns
	}
	return a.buildResult(st, final, reason)
}

func (a *Agent) handleRunError(ctx context.Context, st runState, err error) (*Result, error) {
	_ = a.emitHook(ctx, ErrorEvent{Err: err, Source: "agent"})
	return a.resultForError(st, err), err
}

func addUsage(a, b llm.Usage) llm.Usage {
	return llm.Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CachedTokens: a.CachedTokens + b.CachedTokens, ReasoningTokens: a.ReasoningTokens + b.ReasoningTokens}
}

func totalTokens(u llm.Usage) int {
	if u.TotalTokens() > 0 {
		return u.TotalTokens()
	}
	return u.InputTokens + u.OutputTokens
}
