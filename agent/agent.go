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

// emitFunc receives a stream event. It returns a non-nil error only when the consumer can no
// longer receive (context cancellation / backpressure abort). The turn loop checks this error and
// aborts before doing further work at every point that precedes more model or tool work; the
// per-tool goroutines in executeTools treat their emits as fire-and-forget (the send already
// unblocks on context cancellation, and the loop's next abort gate stops the run).
type emitFunc func(AgentEvent) error

// llmCallFunc performs one model turn. Run uses a buffered [llm.Provider.Execute]; Stream uses
// [Agent.streamLLM], which forwards provider deltas as [LLMDelta] events before returning the
// assembled response.
type llmCallFunc func(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)

// Agent orchestrates LLM turns, tool calls, and memory.
//
// Agent implements component.Component (Start/Stop/Health) so bootstrap auto-wires it as infrastructure and surfaces it in the startup summary.
type Agent struct {
	config    Config
	lifecycle ai.Lifecycle
}

// Run executes the agent loop to completion and returns the final result. It is the buffered
// (non-streaming) view over the same driver that powers [Agent.Stream].
func (a *Agent) Run(ctx context.Context, messages []chat.Message) (*Result, error) {
	a.lifecycle.Touch()
	ctx, cancel := context.WithTimeout(ctx, a.config.WallClock)
	defer cancel()
	llmCall := func(callCtx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
		return a.config.Provider.Execute(callCtx, req)
	}
	return a.drive(ctx, messages, llmCall, func(AgentEvent) error { return nil })
}

// Stream executes the agent loop, emitting the full turn lifecycle as [AgentEvent] values on a
// bounded channel (capacity Config.StreamBuffer). Sends honor the WallClock-bounded context, so
// a canceled context, an expired wall clock, or a stalled consumer stops the loop and closes the
// channel. On a clean run the terminal event is a [RunComplete]; if the consumer stops reading,
// the loop is aborted and the channel is closed without a guaranteed terminal event.
func (a *Agent) Stream(ctx context.Context, messages []chat.Message) (<-chan AgentEvent, error) {
	a.lifecycle.Touch()
	ctx, cancel := context.WithTimeout(ctx, a.config.WallClock)
	ch := make(chan AgentEvent, a.config.StreamBuffer)
	emit := func(evt AgentEvent) error {
		select {
		case ch <- evt:
			return nil
		case <-ctx.Done():
			return mapContextErr(ctx)
		}
	}
	llmCall := func(callCtx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
		return a.streamLLM(callCtx, req, emit)
	}
	go func() {
		defer cancel()
		defer close(ch)
		_, _ = a.drive(ctx, messages, llmCall, emit)
	}()
	return ch, nil
}

// streamLLM runs one model turn in streaming mode: it forwards every provider event as an
// [LLMDelta] and returns the response assembled by the terminal [llm.MessageComplete].
func (a *Agent) streamLLM(ctx context.Context, req llm.CompletionRequest, emit emitFunc) (llm.CompletionResponse, error) {
	streamCh, err := a.config.Provider.Stream(ctx, req)
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	var (
		resp     llm.CompletionResponse
		haveResp bool
	)
	for {
		select {
		case <-ctx.Done():
			return llm.CompletionResponse{}, mapContextErr(ctx)
		case event, ok := <-streamCh:
			if !ok {
				if !haveResp {
					return llm.CompletionResponse{}, ErrStreamIncomplete
				}
				return resp, nil
			}
			if emitErr := emit(LLMDelta{Event: event}); emitErr != nil {
				return llm.CompletionResponse{}, emitErr
			}
			_ = a.emitHook(ctx, StreamObservedEvent{Event: event})
			switch e := event.(type) {
			case llm.MessageComplete:
				resp = e.Response
				haveResp = true
			case llm.StreamError:
				if e.Err != nil {
					return llm.CompletionResponse{}, e.Err
				}
				return llm.CompletionResponse{}, e
			}
		}
	}
}

// drive runs the turn loop and guarantees a terminal RunComplete on every path.
func (a *Agent) drive(ctx context.Context, messages []chat.Message, llmCall llmCallFunc, emit emitFunc) (*Result, error) {
	result, err := a.runLoop(ctx, messages, llmCall, emit)
	_ = emit(RunComplete{Result: result, Err: err})
	return result, err
}

//nolint:gocyclo // Cohesive single turn-orchestration loop; splitting would obscure control flow.
func (a *Agent) runLoop(ctx context.Context, messages []chat.Message, llmCall llmCallFunc, emit emitFunc) (*Result, error) {
	ctx, runSpan := observability.StartNamedSpan(ctx, tracerName, "agent.run",
		observability.WithSpanKind(observability.SpanKindInternal),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, "agent"),
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpAgentRun),
			observability.StringAttribute(semconv.GenAIRequestModel, a.config.Model),
		),
	)
	defer runSpan.End()
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
		if err := emit(TurnStart{Turn: turn}); err != nil {
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
		resp, err := llmCall(turnCtx, req)
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
			_ = emit(TurnComplete{Turn: turn, Message: resp.Message, Usage: resp.Usage})
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
		for _, msg := range a.executeTools(turnCtx, resp.Message.ToolCalls, emit) {
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
			newTokens := a.config.Provider.CountTokens(msgs)
			_ = a.emitHook(turnCtx, ContextCompacted{OldTokens: oldTokens, NewTokens: newTokens, Strategy: fmt.Sprintf("%T", a.config.Compaction)})
			if err := emit(Compacted{OldTokens: oldTokens, NewTokens: newTokens}); err != nil {
				turnSpan.End()
				a.persistHistory(turnCtx, msgs)
				return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, err), err
			}
		}
		_ = a.emitHookErr(turnCtx, StepCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage})
		if err := emit(TurnComplete{Turn: turn, Message: resp.Message, Usage: resp.Usage}); err != nil {
			turnSpan.End()
			a.persistHistory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, err), err
		}
		turnSpan.End()
	}
	a.persistHistory(ctx, msgs)
	_ = a.emitHook(ctx, StopEvent{Reason: StopMaxTurns, Err: ErrMaxTurnsExceeded})
	return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: a.config.MaxTurns}, ErrMaxTurnsExceeded), ErrMaxTurnsExceeded
}
