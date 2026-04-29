package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

const defaultMaxTurns = 100

// ErrContextExceeded is returned when the context window is exceeded
// and the ContextStrategy is FailStrategy.
var ErrContextExceeded = errors.New("agent: context window exceeded")

// Config configures the agent loop.
type Config struct {
	// Provider is the LLM provider to use for completions.
	Provider llm.Provider
	// Tools is the tool registry available to the agent.
	Tools *tool.Registry
	// ToolFormatter, when set, transforms tool results before they are sent
	// to the LLM as tool result messages.
	ToolFormatter tool.Formatter
	// Hooks is an optional hook registry for lifecycle events.
	Hooks *hook.Registry
	// SystemPrompt is prepended to every LLM call.
	// Ignored when SystemPromptTemplate is set.
	SystemPrompt string
	// SystemPromptTemplate, when set, is rendered with SystemPromptData
	// on each LLM call to produce the system prompt. Takes precedence
	// over the plain SystemPrompt string.
	SystemPromptTemplate *PromptTemplate
	// SystemPromptData is the data passed to SystemPromptTemplate.Render.
	SystemPromptData any
	// Model overrides the provider's default model for all LLM calls.
	// When empty the provider's default model is used.
	Model string
	// MaxTurns limits the number of LLM calls (default: 100).
	MaxTurns int
	// MaxTokenBudget limits total input+output tokens (0 = unlimited).
	MaxTokenBudget int
	// ContextStrategy handles context window overflow (default: FailStrategy).
	ContextStrategy ContextStrategy
	// ParallelTools enables concurrent execution of read-only tools.
	// When true and all tool calls in a batch have ReadOnly=true, they
	// execute concurrently. Mixed batches always run sequentially.
	ParallelTools bool
	// Commands is an optional slash command registry.
	// When set, user messages starting with "/" are intercepted and
	// handled as commands before reaching the LLM.
	Commands *CommandRegistry
	// Memory is an optional conversation memory store.
	// When set alongside SessionID, the agent loads history before the run
	// and saves the full conversation after completion.
	Memory Memory
	// SessionID identifies the conversation session for Memory.
	SessionID string
}

// Agent orchestrates the LLM conversation loop with tool execution.
type Agent struct {
	config Config
}

// New creates a new Agent with the given configuration.
func New(config Config) *Agent {
	if config.MaxTurns <= 0 {
		config.MaxTurns = defaultMaxTurns
	}
	if config.ContextStrategy == nil {
		config.ContextStrategy = FailStrategy{}
	}
	return &Agent{config: config}
}

// Run executes the agent loop synchronously, returning the final result.
func (a *Agent) Run(ctx context.Context, messages []llm.Message) (*Result, error) {
	msgs := make([]llm.Message, len(messages))
	copy(msgs, messages)

	// Check for slash commands before the LLM loop.
	if result, handled := a.handleCommand(ctx, msgs); handled {
		return result, nil
	}

	// Load conversation history from memory.
	if a.config.Memory != nil && a.config.SessionID != "" {
		history, err := a.config.Memory.Load(ctx, a.config.SessionID)
		if err != nil {
			return nil, fmt.Errorf("agent: failed to load memory: %w", err)
		}
		if len(history) > 0 {
			msgs = append(history, msgs...)
			a.emitHook(ctx, MemoryLoaded{SessionID: a.config.SessionID, MessageCount: len(history)})
		}
	}

	var totalUsage llm.Usage

	for turn := 1; turn <= a.config.MaxTurns; turn++ {
		// Emit TurnStart hook
		if hr := a.emitHook(ctx, TurnStart{Turn: turn}); hr.Action == hook.ActionAbort {
			a.saveMemory(ctx, msgs)
			return a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted), nil
		}

		// Build completion request
		req := a.buildRequest(msgs)

		// Emit PreLLMCall hook
		if hr := a.emitHook(ctx, PreLLMCall{Request: req}); hr.Action == hook.ActionAbort {
			a.saveMemory(ctx, msgs)
			return a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted), nil
		} else if hr.Action == hook.ActionModify {
			if modified, ok := hr.ModifiedData.(llm.CompletionRequest); ok {
				req = modified
			}
		}

		// Call LLM
		resp, err := a.config.Provider.Complete(ctx, req)
		if err != nil {
			a.emitHook(ctx, OnError{Err: err, Source: "llm_provider"})
			return nil, fmt.Errorf("agent: llm call failed on turn %d: %w", turn, err)
		}

		// Emit PostLLMCall hook
		a.emitHook(ctx, PostLLMCall{Response: *resp})

		// Accumulate usage
		totalUsage = addUsage(totalUsage, resp.Usage)

		// Append assistant message
		msgs = append(msgs, resp.Message)

		// If no tool calls, we're done
		if !resp.HasToolCalls() {
			a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
			a.saveMemory(ctx, msgs)
			return a.buildResult(msgs, resp.Message, totalUsage, turn, StopEndTurn), nil
		}

		// Execute tool calls
		toolResults := a.executeTools(ctx, resp.Message.ToolCalls)
		for _, tr := range toolResults {
			msgs = append(msgs, llm.ToolResultMsg(tr.id, tr.content, tr.isError))
		}

		// Check token budget
		if a.config.MaxTokenBudget > 0 && totalTokens(totalUsage) >= a.config.MaxTokenBudget {
			a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
			a.saveMemory(ctx, msgs)
			return a.buildResult(msgs, resp.Message, totalUsage, turn, StopMaxBudget), nil
		}

		// Check if context is too large and compact
		if a.contextTooLarge(msgs) {
			oldTokens := a.config.Provider.CountTokens(msgs)
			compacted, compactErr := a.config.ContextStrategy.Compact(msgs, a.config.Provider.Capabilities().MaxContextTokens)
			if compactErr != nil {
				return nil, fmt.Errorf("agent: context compaction failed: %w", compactErr)
			}
			msgs = compacted
			newTokens := a.config.Provider.CountTokens(msgs)
			a.emitHook(ctx, ContextCompacted{OldTokens: oldTokens, NewTokens: newTokens, Strategy: fmt.Sprintf("%T", a.config.ContextStrategy)})
		}

		// Emit TurnEnd hook
		a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
	}

	// Reached max turns
	var finalMsg llm.AssistantMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		if am, ok := msgs[i].(llm.AssistantMessage); ok {
			finalMsg = am
			break
		}
	}
	a.saveMemory(ctx, msgs)
	return a.buildResult(msgs, finalMsg, totalUsage, a.config.MaxTurns, StopMaxTurns), nil
}

// Stream executes the agent loop and sends events to the returned channel.
func (a *Agent) Stream(ctx context.Context, messages []llm.Message) (<-chan Event, error) {
	ch := make(chan Event, 16)

	go func() {
		defer close(ch)

		// send delivers an event respecting ctx cancellation. Returns false if
		// the caller has stopped reading or ctx was canceled, in which case
		// the goroutine should unwind.
		send := func(evt Event) bool {
			select {
			case ch <- evt:
				return true
			case <-ctx.Done():
				return false
			}
		}

		msgs := make([]llm.Message, len(messages))
		copy(msgs, messages)

		// Check for slash commands before the LLM loop.
		if result, handled := a.handleCommand(ctx, msgs); handled {
			send(CompleteEvent{Result: *result})
			return
		}

		// Load conversation history from memory.
		if a.config.Memory != nil && a.config.SessionID != "" {
			history, err := a.config.Memory.Load(ctx, a.config.SessionID)
			if err == nil && len(history) > 0 {
				msgs = append(history, msgs...)
				a.emitHook(ctx, MemoryLoaded{SessionID: a.config.SessionID, MessageCount: len(history)})
			}
		}

		var totalUsage llm.Usage

		for turn := 1; turn <= a.config.MaxTurns; turn++ {
			if ctx.Err() != nil {
				return
			}

			if !send(TurnStartEvent{Turn: turn}) {
				return
			}

			if hr := a.emitHook(ctx, TurnStart{Turn: turn}); hr.Action == hook.ActionAbort {
				a.saveMemory(ctx, msgs)
				send(CompleteEvent{Result: *a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted)})
				return
			}

			req := a.buildRequest(msgs)

			if hr := a.emitHook(ctx, PreLLMCall{Request: req}); hr.Action == hook.ActionAbort {
				a.saveMemory(ctx, msgs)
				send(CompleteEvent{Result: *a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted)})
				return
			} else if hr.Action == hook.ActionModify {
				if modified, ok := hr.ModifiedData.(llm.CompletionRequest); ok {
					req = modified
				}
			}

			// Use streaming if provider supports it
			streamCh, err := a.config.Provider.Stream(ctx, req)
			if err != nil {
				a.emitHook(ctx, OnError{Err: err, Source: "llm_provider"})
				return
			}

			// Collect the streamed response
			var resp *llm.CompletionResponse
			for event := range streamCh {
				if !send(LLMStreamEvent{Event: event}) {
					return
				}
				if mc, ok := event.(llm.MessageComplete); ok {
					resp = &mc.Response
				}
			}

			if resp == nil {
				return
			}

			a.emitHook(ctx, PostLLMCall{Response: *resp})

			totalUsage = addUsage(totalUsage, resp.Usage)
			msgs = append(msgs, resp.Message)

			if !resp.HasToolCalls() {
				a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
				if !send(TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}) {
					return
				}
				a.saveMemory(ctx, msgs)
				send(CompleteEvent{Result: *a.buildResult(msgs, resp.Message, totalUsage, turn, StopEndTurn)})
				return
			}

			// Emit executing events for all tools
			for _, tc := range resp.Message.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if !send(ToolExecutingEvent{ToolUseID: tc.ID, Name: tc.Function.Name, Input: input}) {
					return
				}
			}

			// Execute tools (parallel when safe)
			toolResults := a.executeTools(ctx, resp.Message.ToolCalls)
			for _, tr := range toolResults {
				if !send(ToolCompleteEvent{ToolUseID: tr.id, Name: tr.name, Result: tr.result, Err: tr.err}) {
					return
				}
				msgs = append(msgs, llm.ToolResultMsg(tr.id, tr.content, tr.isError))
			}

			if a.config.MaxTokenBudget > 0 && totalTokens(totalUsage) >= a.config.MaxTokenBudget {
				a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
				if !send(TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}) {
					return
				}
				a.saveMemory(ctx, msgs)
				send(CompleteEvent{Result: *a.buildResult(msgs, resp.Message, totalUsage, turn, StopMaxBudget)})
				return
			}

			if a.contextTooLarge(msgs) {
				oldTokens := a.config.Provider.CountTokens(msgs)
				compacted, compactErr := a.config.ContextStrategy.Compact(msgs, a.config.Provider.Capabilities().MaxContextTokens)
				if compactErr != nil {
					return
				}
				msgs = compacted
				newTokens := a.config.Provider.CountTokens(msgs)
				a.emitHook(ctx, ContextCompacted{OldTokens: oldTokens, NewTokens: newTokens, Strategy: fmt.Sprintf("%T", a.config.ContextStrategy)})
				if !send(ContextCompactedEvent{OldTokens: oldTokens, NewTokens: newTokens}) {
					return
				}
			}

			a.emitHook(ctx, TurnEnd{Turn: turn, Message: resp.Message})
			if !send(TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}) {
				return
			}
		}

		var finalMsg llm.AssistantMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			if am, ok := msgs[i].(llm.AssistantMessage); ok {
				finalMsg = am
				break
			}
		}
		a.saveMemory(ctx, msgs)
		send(CompleteEvent{Result: *a.buildResult(msgs, finalMsg, totalUsage, a.config.MaxTurns, StopMaxTurns)})
	}()

	return ch, nil
}

// --- internal helpers ---

func (a *Agent) buildRequest(msgs []llm.Message) llm.CompletionRequest {
	req := llm.CompletionRequest{
		Messages: msgs,
	}
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
		req.Tools = a.config.Tools.List()
	}
	return req
}

func (a *Agent) executeTool(ctx context.Context, tc llm.ToolCall) (*tool.Result, error) {
	if a.config.Tools == nil {
		return nil, fmt.Errorf("tool %q not found: no tool registry", tc.Function.Name)
	}

	// Emit PreToolCall hook
	input := json.RawMessage(tc.Function.Arguments)
	if hr := a.emitHook(ctx, PreToolCall{Name: tc.Function.Name, Input: input}); hr.Action == hook.ActionAbort {
		return nil, fmt.Errorf("tool %q aborted by hook: %s", tc.Function.Name, hr.Reason)
	}

	callable, ok := a.config.Tools.Get(tc.Function.Name)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", tc.Function.Name)
	}

	toolCtx := tool.NewContext(ctx)
	toolCtx.ToolUseID = tc.ID

	result, err := callable.Call(toolCtx, input)

	// Apply formatter to transform result content for the LLM.
	if err == nil && result != nil && a.config.ToolFormatter != nil {
		if formatted, fmtErr := a.config.ToolFormatter.Format(tc.Function.Name, result); fmtErr == nil {
			result.Content = formatted
		}
	}

	// Emit PostToolCall hook
	a.emitHook(ctx, PostToolCall{
		Name:   tc.Function.Name,
		Input:  input,
		Result: result,
		Err:    err,
	})

	return result, err
}

// toolResultEntry holds the result of a single tool execution.
type toolResultEntry struct {
	id      string
	name    string
	result  *tool.Result
	err     error
	content string
	isError bool
}

// allReadOnly returns true if every tool call targets a ReadOnly tool.
func (a *Agent) allReadOnly(calls []llm.ToolCall) bool {
	if a.config.Tools == nil {
		return false
	}
	for _, tc := range calls {
		c, ok := a.config.Tools.Get(tc.Function.Name)
		if !ok || !c.Definition().ReadOnly {
			return false
		}
	}
	return true
}

// executeTools runs tool calls sequentially or in parallel depending on
// Config.ParallelTools and whether all tools are read-only.
func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall) []toolResultEntry {
	results := make([]toolResultEntry, len(calls))

	if a.config.ParallelTools && len(calls) > 1 && a.allReadOnly(calls) {
		names := make([]string, len(calls))
		for i, tc := range calls {
			names[i] = tc.Function.Name
		}
		a.emitHook(ctx, ToolsParallelized{ToolNames: names, Count: len(calls)})

		var wg sync.WaitGroup
		for i, tc := range calls {
			wg.Add(1)
			go func(idx int, tc llm.ToolCall) {
				defer wg.Done()
				r, err := a.executeTool(ctx, tc)
				entry := toolResultEntry{id: tc.ID, name: tc.Function.Name, result: r, err: err}
				entry.isError = err != nil
				if err != nil {
					entry.content = err.Error()
				} else if r != nil {
					entry.content = r.Content
				}
				results[idx] = entry
			}(i, tc)
		}
		wg.Wait()
	} else {
		for i, tc := range calls {
			r, err := a.executeTool(ctx, tc)
			entry := toolResultEntry{id: tc.ID, name: tc.Function.Name, result: r, err: err}
			entry.isError = err != nil
			if err != nil {
				entry.content = err.Error()
			} else if r != nil {
				entry.content = r.Content
			}
			results[i] = entry
		}
	}

	return results
}

func (a *Agent) emitHook(ctx context.Context, event hook.Event) hook.Result {
	if a.config.Hooks == nil {
		return hook.Continue()
	}
	return a.config.Hooks.Emit(ctx, event)
}

func (a *Agent) contextTooLarge(msgs []llm.Message) bool {
	caps := a.config.Provider.Capabilities()
	if caps.MaxContextTokens <= 0 {
		return false
	}
	tokens := a.config.Provider.CountTokens(msgs)
	return tokens > caps.MaxContextTokens
}

func (a *Agent) saveMemory(ctx context.Context, msgs []llm.Message) {
	if a.config.Memory != nil && a.config.SessionID != "" {
		_ = a.config.Memory.Save(ctx, a.config.SessionID, msgs)
	}
}

func (a *Agent) buildResult(msgs []llm.Message, finalMsg llm.AssistantMessage, usage llm.Usage, turns int, reason StopReason) *Result {
	return &Result{
		Messages:     msgs,
		FinalMessage: finalMsg,
		TotalUsage:   usage,
		TurnCount:    turns,
		StopReason:   reason,
	}
}

func addUsage(a, b llm.Usage) llm.Usage {
	return llm.Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
		CacheReadTokens:  a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
		ThinkingTokens:   a.ThinkingTokens + b.ThinkingTokens,
	}
}

func totalTokens(u llm.Usage) int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.PromptTokens + u.CompletionTokens
}
