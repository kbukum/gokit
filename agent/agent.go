package agent

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/observability"
)

const tracerName = "github.com/kbukum/gokit/agent"

// Agent orchestrates LLM turns, tool calls, and memory.
//
// Agent implements component.Component (Start/Stop/Health) so bootstrap auto-wires it as infrastructure and surfaces it in the startup summary.
type Agent struct {
	config    Config
	lifecycle ai.Lifecycle
}

func (a *Agent) Run(ctx context.Context, messages []chat.Message) (*Result, error) {
	a.lifecycle.Touch()
	ctx, runSpan := observability.StartNamedSpan(ctx, tracerName, "agent.run",
		observability.WithSpanKind(observability.SpanKindInternal),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, "agent"),
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpAgentRun),
			observability.StringAttribute(semconv.GenAIRequestModel, a.config.Model),
		),
	)
	defer runSpan.End()
	ctx, cancel := context.WithTimeout(ctx, a.config.WallClock)
	defer cancel()
	msgs := append([]chat.Message(nil), messages...)
	if result, handled := a.handleCommand(ctx, msgs); handled {
		return result, nil
	}
	if a.config.Store != nil && a.config.SessionID != "" {
		history, err := a.config.Store.Load(ctx, a.config.SessionID)
		if err != nil {
			return nil, fmt.Errorf("agent: failed to load memory: %w", err)
		}
		if len(history) > 0 {
			msgs = append(history, msgs...)
			_ = a.emitHook(ctx, MemoryLoaded{SessionID: a.config.SessionID, MessageCount: len(history)})
		}
	}
	var totalUsage llm.Usage
	toolCalls := 0
	for turn := 1; turn <= a.config.MaxTurns; turn++ {
		turnCtx, turnSpan := observability.StartNamedSpan(ctx, tracerName, "agent.turn",
			observability.WithSpanKind(observability.SpanKindInternal),
			observability.WithSpanAttributes(
				observability.StringAttribute(semconv.GenAIOperationName, semconv.OpAgentTurn),
				observability.IntAttribute("agent.turn", turn),
			),
		)
		if err := a.budgetError(turnCtx, budgetState{usage: totalUsage, turn: turn, toolCalls: toolCalls}); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			a.persistHistory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn - 1}, err), err
		}
		if err := a.emitHookErr(turnCtx, StartEvent{Turn: turn}); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			return nil, err
		}
		req := a.buildRequest(msgs)
		if err := a.emitHookErr(turnCtx, LLMRequestEvent{Request: req}); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			return nil, err
		}
		resp, err := a.config.Provider.Execute(turnCtx, req)
		if err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			return a.handleRunError(turnCtx, runState{msgs: msgs, usage: totalUsage, turns: turn - 1}, fmt.Errorf("agent: llm call failed on turn %d: %w", turn, err))
		}
		_ = a.emitHookErr(turnCtx, LLMResponseEvent{Request: req, Response: &resp})
		totalUsage = addUsage(totalUsage, resp.Usage)
		turnSpan.SetAttributes(
			observability.IntAttribute(semconv.GenAIUsageInputTokens, resp.Usage.InputTokens),
			observability.IntAttribute(semconv.GenAIUsageOutputTokens, resp.Usage.OutputTokens),
		)
		msgs = append(msgs, resp.Message)
		if err := a.budgetError(turnCtx, budgetState{usage: totalUsage, turn: turn, toolCalls: toolCalls}); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			a.persistHistory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, err), err
		}
		if !resp.HasToolCalls() {
			_ = a.emitHookErr(turnCtx, StepCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage})
			turnSpan.End()
			a.persistHistory(turnCtx, msgs)
			reason := resp.StopReason
			if reason == "" {
				reason = StopEndTurn
			}
			_ = a.emitHook(turnCtx, StopEvent{Reason: reason})
			return a.buildResult(runState{msgs: msgs, usage: totalUsage, turns: turn}, resp.Message, reason), nil
		}
		if toolCalls+len(resp.Message.ToolCalls) > a.config.MaxToolCalls {
			turnSpan.End()
			a.persistHistory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, ErrMaxToolCallsExceeded), ErrMaxToolCallsExceeded
		}
		toolCalls += len(resp.Message.ToolCalls)
		for _, msg := range a.executeTools(turnCtx, resp.Message.ToolCalls) {
			msgs = append(msgs, msg)
		}
		if a.contextTooLarge(msgs) {
			oldTokens := a.config.Provider.CountTokens(msgs)
			compacted, compactErr := a.config.Compaction.Compact(turnCtx, msgs, a.config.Provider.Capabilities().MaxInputTokens)
			if compactErr != nil {
				turnSpan.RecordError(compactErr)
				turnSpan.End()
				return nil, fmt.Errorf("agent: context compaction failed: %w", compactErr)
			}
			msgs = compacted
			_ = a.emitHook(turnCtx, ContextCompacted{OldTokens: oldTokens, NewTokens: a.config.Provider.CountTokens(msgs), Strategy: fmt.Sprintf("%T", a.config.Compaction)})
		}
		_ = a.emitHookErr(turnCtx, StepCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage})
		turnSpan.End()
	}
	a.persistHistory(ctx, msgs)
	_ = a.emitHook(ctx, StopEvent{Reason: StopMaxTurns, Err: ErrMaxTurnsExceeded})
	return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: a.config.MaxTurns}, ErrMaxTurnsExceeded), ErrMaxTurnsExceeded
}

func (a *Agent) Stream(ctx context.Context, messages []chat.Message) (<-chan llm.StreamEvent, error) {
	a.lifecycle.Touch()
	ctx, cancel := context.WithTimeout(ctx, a.config.WallClock)
	ch := make(chan llm.StreamEvent, a.config.StreamBuffer)
	go func() {
		defer cancel()
		defer close(ch)
		send := func(evt llm.StreamEvent) bool {
			_ = a.emitHook(ctx, StreamObservedEvent{Event: evt})
			select {
			case ch <- evt:
				return true
			case <-ctx.Done():
				return false
			}
		}
		req := a.buildRequest(append([]chat.Message(nil), messages...))
		streamCh, err := a.config.Provider.Stream(ctx, req)
		if err != nil {
			send(llm.StreamError{Err: err})
			return
		}
		var usage llm.Usage
		for {
			select {
			case <-ctx.Done():
				send(llm.StreamError{Err: mapContextErr(ctx)})
				return
			case event, ok := <-streamCh:
				if !ok {
					return
				}
				if u, ok := event.(llm.UsageDelta); ok {
					usage = llm.Usage(u)
					if err := a.budgetError(ctx, budgetState{usage: usage, turn: 1}); err != nil {
						send(llm.StreamError{Err: err})
						return
					}
				}
				if !send(event) {
					return
				}
			}
		}
	}()
	return ch, nil
}
