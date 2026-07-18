package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/ai/prompt"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/tool"
)

const tracerName = "github.com/kbukum/gokit/agent"

const (
	defaultMaxTurns        = 10
	defaultMaxTokens       = 100_000
	defaultMaxToolCalls    = 50
	defaultToolConcurrency = 4
	defaultStreamBuffer    = 16
)

var ErrContextExceeded = errors.New("agent: context window exceeded")

type Config struct {
	Provider             llm.Provider
	Tools                *tool.Registry
	ToolFormatter        tool.Formatter
	Hooks                *hook.Registry
	SystemPrompt         string
	SystemPromptTemplate *prompt.Template
	SystemPromptData     any
	Model                string
	Budget               ai.Budget
	MaxTurns             int
	MaxTokens            int
	WallClock            time.Duration
	MaxToolCalls         int
	ToolConcurrency      int
	ToolTimeout          time.Duration
	Policy               *resilience.Policy
	MemoryPolicy         MemoryPolicy
	Commands             *CommandRegistry
	Memory               Memory
	SessionID            string
	StreamBuffer         int
}

// Agent orchestrates LLM turns, tool calls, and memory.
//
// Per locked decision D12 (NATIVE COMPONENT),
// Agent implements component.Component (Start/Stop/Health)
// so bootstrap auto-wires it as infrastructure and surfaces it in the startup summary.
type Agent struct {
	config    Config
	lifecycle ai.Lifecycle
}

func New(config Config) *Agent {
	if config.Budget.MaxTokens > 0 {
		config.MaxTokens = config.Budget.MaxTokens
	}
	if config.Budget.MaxCalls > 0 {
		config.MaxToolCalls = config.Budget.MaxCalls
	}
	if config.Budget.WallClock > 0 {
		config.WallClock = config.Budget.WallClock
	}
	if config.MaxTurns <= 0 {
		config.MaxTurns = defaultMaxTurns
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = defaultMaxTokens
	}
	if config.WallClock <= 0 {
		config.WallClock = 60 * time.Second
	}
	if config.MaxToolCalls <= 0 {
		config.MaxToolCalls = defaultMaxToolCalls
	}
	if config.ToolConcurrency <= 0 {
		config.ToolConcurrency = defaultToolConcurrency
	}
	if config.ToolTimeout <= 0 {
		config.ToolTimeout = 30 * time.Second
	}
	if config.MemoryPolicy == nil {
		config.MemoryPolicy = RingBufferPolicy{KeepLast: 20}
	}
	if config.StreamBuffer <= 0 {
		config.StreamBuffer = defaultStreamBuffer
	}
	return &Agent{config: config}
}

// --- component.Component (D12) ---

// Name returns the agent name (configured Model or "agent").
func (a *Agent) Name() string {
	if a.config.Model != "" {
		return "agent-" + a.config.Model
	}
	return "agent"
}

// IsAvailable reports whether the underlying provider is reachable.
func (a *Agent) IsAvailable(ctx context.Context) bool {
	if a.config.Provider == nil {
		return false
	}
	return a.config.Provider.IsAvailable(ctx)
}

// Start marks the agent ready. The underlying provider is started independently by bootstrap;
// the agent itself only flips its lifecycle flag.
func (a *Agent) Start(_ context.Context) error { a.lifecycle.MarkReady(); return nil }

// Stop marks the agent as stopped. Inflight Run calls observe ctx cancellation.
func (a *Agent) Stop(_ context.Context) error { a.lifecycle.MarkStopped(); return nil }

// Health reports the agent's readiness and the underlying provider's reachability.
func (a *Agent) Health(ctx context.Context) component.Health {
	if !a.lifecycle.Ready() {
		return component.Health{Name: a.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if a.config.Provider != nil && !a.config.Provider.IsAvailable(ctx) {
		return component.Health{Name: a.Name(), Status: component.StatusUnhealthy, Message: "provider unreachable"}
	}
	msg := "ready"
	if last := a.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_turn=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: a.Name(), Status: component.StatusHealthy, Message: msg}
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
	if a.config.Memory != nil && a.config.SessionID != "" {
		history, err := a.config.Memory.Load(ctx, a.config.SessionID)
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
		if err := a.budgetError(turnCtx, totalUsage, turn, toolCalls); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			a.saveMemory(turnCtx, msgs)
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
		if err := a.budgetError(turnCtx, totalUsage, turn, toolCalls); err != nil {
			turnSpan.RecordError(err)
			turnSpan.End()
			a.saveMemory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, err), err
		}
		if !resp.HasToolCalls() {
			_ = a.emitHookErr(turnCtx, StepCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage})
			turnSpan.End()
			a.saveMemory(turnCtx, msgs)
			reason := resp.StopReason
			if reason == "" {
				reason = StopEndTurn
			}
			_ = a.emitHook(turnCtx, StopEvent{Reason: reason})
			return a.buildResult(runState{msgs: msgs, usage: totalUsage, turns: turn}, resp.Message, reason), nil
		}
		if toolCalls+len(resp.Message.ToolCalls) > a.config.MaxToolCalls {
			turnSpan.End()
			a.saveMemory(turnCtx, msgs)
			return a.resultForError(runState{msgs: msgs, usage: totalUsage, turns: turn}, ErrMaxToolCallsExceeded), ErrMaxToolCallsExceeded
		}
		toolCalls += len(resp.Message.ToolCalls)
		for _, msg := range a.executeTools(turnCtx, resp.Message.ToolCalls) {
			msgs = append(msgs, msg)
		}
		if a.contextTooLarge(msgs) {
			oldTokens := a.config.Provider.CountTokens(msgs)
			compacted, compactErr := a.config.MemoryPolicy.Compact(turnCtx, msgs, a.config.Provider.Capabilities().MaxInputTokens)
			if compactErr != nil {
				turnSpan.RecordError(compactErr)
				turnSpan.End()
				return nil, fmt.Errorf("agent: context compaction failed: %w", compactErr)
			}
			msgs = compacted
			_ = a.emitHook(turnCtx, ContextCompacted{OldTokens: oldTokens, NewTokens: a.config.Provider.CountTokens(msgs), Strategy: fmt.Sprintf("%T", a.config.MemoryPolicy)})
		}
		_ = a.emitHookErr(turnCtx, StepCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage})
		turnSpan.End()
	}
	a.saveMemory(ctx, msgs)
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
					if err := a.budgetError(ctx, usage, 1, 0); err != nil {
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

func (a *Agent) buildRequest(msgs []chat.Message) llm.CompletionRequest {
	req := llm.CompletionRequest{Messages: msgs}
	if a.config.SystemPromptTemplate != nil {
		if rendered, err := a.config.SystemPromptTemplate.Render(a.config.SystemPromptData); err == nil {
			req.SystemPrompt = rendered
		}
	} else if a.config.SystemPrompt != "" {
		req.SystemPrompt = a.config.SystemPrompt
	}
	if a.config.Model != "" {
		req.Model = a.config.Model
	}
	if a.config.Tools != nil {
		req.Tools = a.config.Tools.ToolSpecs()
	}
	return req
}

func (a *Agent) executeTool(ctx context.Context, tc ai.ToolUseBlock) (*tool.Result, error) {
	if a.config.Tools == nil {
		return nil, fmt.Errorf("tool %q not found: no tool registry", tc.Name)
	}
	input := ai.NormalizeToolInput(tc.Input)
	if hookErr := a.emitHookErr(ctx, ToolCallEvent{ToolUseID: tc.ID, Name: tc.Name, Input: input}); hookErr != nil {
		return nil, hookErr
	}
	callable, ok := a.config.Tools.Get(tc.Name)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", tc.Name)
	}
	result, err := resilience.Execute(ctx, a.toolPolicy(tc.Name), func(callCtx context.Context) (*tool.Result, error) {
		toolCtx := tool.NewContext(callCtx)
		toolCtx.ToolUseID = tc.ID
		if a.config.ToolTimeout > 0 {
			var cancel context.CancelFunc
			toolCtx, cancel = toolCtx.WithTimeout(a.config.ToolTimeout)
			defer cancel()
		}
		return callable.Call(toolCtx, input)
	})
	if err == nil && result != nil && a.config.ToolFormatter != nil {
		if formatted, fmtErr := a.config.ToolFormatter.Format(tc.Name, result); fmtErr == nil {
			result.Content = formatted
		}
	}
	_ = a.emitHookErr(ctx, ToolResultEvent{ToolUseID: tc.ID, Name: tc.Name, Input: input, Result: result, Err: err})
	return result, err
}

func (a *Agent) toolPolicy(name string) *resilience.Policy {
	if a.config.Tools != nil {
		if p := a.config.Tools.PolicyFor(name); p != nil {
			return p
		}
	}
	return a.config.Policy
}

func (a *Agent) executeTools(ctx context.Context, calls []ai.ToolUseBlock) []chat.ToolResultMessage {
	results := make([]chat.ToolResultMessage, len(calls))
	sem := make(chan struct{}, a.config.ToolConcurrency)
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, tc ai.ToolUseBlock) {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := a.executeTool(ctx, tc)
			blk := tool.ResultBlock(tc.ID, r, err)
			results[idx] = chat.ToolResultMsg(blk.ID, blk.Content, blk.IsError)
		}(i, tc)
	}
	wg.Wait()
	return results
}

func (a *Agent) emitHook(ctx context.Context, event hook.Event) error {
	if a.config.Hooks == nil {
		return nil
	}
	return a.config.Hooks.Emit(ctx, event)
}

func (a *Agent) emitHookErr(ctx context.Context, event hook.Event) error {
	err := a.emitHook(ctx, event)
	if err == nil {
		return nil
	}
	_ = a.emitHook(ctx, ErrorEvent{Err: err, Source: string(event.Type())})
	if errors.Is(err, hook.ErrFatalHook) {
		return err
	}
	return nil
}

func (a *Agent) contextTooLarge(msgs []chat.Message) bool {
	caps := a.config.Provider.Capabilities()
	return caps.MaxInputTokens > 0 && a.config.Provider.CountTokens(msgs) > caps.MaxInputTokens
}

func (a *Agent) saveMemory(ctx context.Context, msgs []chat.Message) {
	if a.config.Memory != nil && a.config.SessionID != "" {
		_ = a.config.Memory.Save(ctx, a.config.SessionID, msgs)
	}
}

// runState is the evolving progress of a Run loop: accumulated messages, token usage,
// and completed turn count.
type runState struct {
	msgs  []chat.Message
	usage llm.Usage
	turns int
}

func (a *Agent) buildResult(st runState, finalMsg chat.AssistantMessage, reason StopReason) *Result {
	return &Result{Messages: st.msgs, FinalMessage: finalMsg, TotalUsage: st.usage, TurnCount: st.turns, StopReason: reason}
}

func (a *Agent) budgetError(ctx context.Context, usage llm.Usage, turn, toolCalls int) error {
	if err := ctx.Err(); err != nil {
		return mapContextErr(ctx)
	}
	if a.config.MaxToolCalls > 0 && toolCalls > a.config.MaxToolCalls {
		return ErrMaxToolCallsExceeded
	}
	if a.config.MaxTokens > 0 && totalTokens(usage) >= a.config.MaxTokens {
		return ErrMaxTokensExceeded
	}
	if turn > a.config.MaxTurns {
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
