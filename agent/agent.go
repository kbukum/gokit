package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	// Hooks is an optional hook registry for lifecycle events.
	Hooks *hook.Registry
	// SystemPrompt is prepended to every LLM call.
	SystemPrompt string
	// MaxTurns limits the number of LLM calls (default: 100).
	MaxTurns int
	// MaxTokenBudget limits total input+output tokens (0 = unlimited).
	MaxTokenBudget int
	// ContextStrategy handles context window overflow (default: FailStrategy).
	ContextStrategy ContextStrategy
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

	var totalUsage llm.Usage

	for turn := 1; turn <= a.config.MaxTurns; turn++ {
		// Emit TurnStart hook
		if hr := a.emitHook(TurnStart{Turn: turn}); hr.Action == hook.ActionAbort {
			return a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted), nil
		}

		// Build completion request
		req := a.buildRequest(msgs)

		// Emit PreLLMCall hook
		if hr := a.emitHook(PreLLMCall{Request: req}); hr.Action == hook.ActionAbort {
			return a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted), nil
		} else if hr.Action == hook.ActionModify {
			if modified, ok := hr.ModifiedData.(llm.CompletionRequest); ok {
				req = modified
			}
		}

		// Call LLM
		resp, err := a.config.Provider.Complete(ctx, req)
		if err != nil {
			a.emitHook(OnError{Err: err, Source: "llm_provider"})
			return nil, fmt.Errorf("agent: llm call failed on turn %d: %w", turn, err)
		}

		// Emit PostLLMCall hook
		a.emitHook(PostLLMCall{Response: *resp})

		// Accumulate usage
		totalUsage = addUsage(totalUsage, resp.Usage)

		// Append assistant message
		msgs = append(msgs, resp.Message)

		// If no tool calls, we're done
		if !resp.HasToolCalls() {
			a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
			return a.buildResult(msgs, resp.Message, totalUsage, turn, StopEndTurn), nil
		}

		// Execute tool calls
		for _, tc := range resp.Message.ToolCalls {
			toolResult, toolErr := a.executeTool(ctx, tc)
			isError := toolErr != nil
			content := ""
			if toolErr != nil {
				content = toolErr.Error()
			} else if toolResult != nil {
				content = toolResult.Content
			}

			msgs = append(msgs, llm.ToolResultMsg(tc.ID, content, isError))
		}

		// Check token budget
		if a.config.MaxTokenBudget > 0 && totalTokens(totalUsage) >= a.config.MaxTokenBudget {
			a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
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
			_ = oldTokens
			_ = newTokens
		}

		// Emit TurnEnd hook
		a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
	}

	// Reached max turns
	var finalMsg llm.AssistantMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		if am, ok := msgs[i].(llm.AssistantMessage); ok {
			finalMsg = am
			break
		}
	}
	return a.buildResult(msgs, finalMsg, totalUsage, a.config.MaxTurns, StopMaxTurns), nil
}

// Stream executes the agent loop and sends events to the returned channel.
func (a *Agent) Stream(ctx context.Context, messages []llm.Message) (<-chan Event, error) {
	ch := make(chan Event, 16)

	go func() {
		defer close(ch)

		msgs := make([]llm.Message, len(messages))
		copy(msgs, messages)

		var totalUsage llm.Usage

		for turn := 1; turn <= a.config.MaxTurns; turn++ {
			if ctx.Err() != nil {
				return
			}

			ch <- TurnStartEvent{Turn: turn}

			if hr := a.emitHook(TurnStart{Turn: turn}); hr.Action == hook.ActionAbort {
				ch <- CompleteEvent{Result: *a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted)}
				return
			}

			req := a.buildRequest(msgs)

			if hr := a.emitHook(PreLLMCall{Request: req}); hr.Action == hook.ActionAbort {
				ch <- CompleteEvent{Result: *a.buildResult(msgs, llm.AssistantMessage{}, totalUsage, turn-1, StopAborted)}
				return
			} else if hr.Action == hook.ActionModify {
				if modified, ok := hr.ModifiedData.(llm.CompletionRequest); ok {
					req = modified
				}
			}

			// Use streaming if provider supports it
			streamCh, err := a.config.Provider.Stream(ctx, req)
			if err != nil {
				a.emitHook(OnError{Err: err, Source: "llm_provider"})
				return
			}

			// Collect the streamed response
			var resp *llm.CompletionResponse
			for event := range streamCh {
				ch <- LLMStreamEvent{Event: event}
				if mc, ok := event.(llm.MessageComplete); ok {
					resp = &mc.Response
				}
			}

			if resp == nil {
				return
			}

			a.emitHook(PostLLMCall{Response: *resp})

			totalUsage = addUsage(totalUsage, resp.Usage)
			msgs = append(msgs, resp.Message)

			if !resp.HasToolCalls() {
				a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
				ch <- TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}
				ch <- CompleteEvent{Result: *a.buildResult(msgs, resp.Message, totalUsage, turn, StopEndTurn)}
				return
			}

			for _, tc := range resp.Message.ToolCalls {
				// Arguments is already a JSON string — use it directly as RawMessage
				// to avoid double-encoding.
				input := json.RawMessage(tc.Function.Arguments)
				ch <- ToolExecutingEvent{ToolUseID: tc.ID, Name: tc.Function.Name, Input: input}

				toolResult, toolErr := a.executeTool(ctx, tc)
				ch <- ToolCompleteEvent{ToolUseID: tc.ID, Name: tc.Function.Name, Result: toolResult, Err: toolErr}

				isError := toolErr != nil
				content := ""
				if toolErr != nil {
					content = toolErr.Error()
				} else if toolResult != nil {
					content = toolResult.Content
				}
				msgs = append(msgs, llm.ToolResultMsg(tc.ID, content, isError))
			}

			if a.config.MaxTokenBudget > 0 && totalTokens(totalUsage) >= a.config.MaxTokenBudget {
				a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
				ch <- TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}
				ch <- CompleteEvent{Result: *a.buildResult(msgs, resp.Message, totalUsage, turn, StopMaxBudget)}
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
				ch <- ContextCompactedEvent{OldTokens: oldTokens, NewTokens: newTokens}
			}

			a.emitHook(TurnEnd{Turn: turn, Message: resp.Message})
			ch <- TurnCompleteEvent{Turn: turn, Message: resp.Message, Usage: resp.Usage}
		}

		var finalMsg llm.AssistantMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			if am, ok := msgs[i].(llm.AssistantMessage); ok {
				finalMsg = am
				break
			}
		}
		ch <- CompleteEvent{Result: *a.buildResult(msgs, finalMsg, totalUsage, a.config.MaxTurns, StopMaxTurns)}
	}()

	return ch, nil
}

// --- internal helpers ---

func (a *Agent) buildRequest(msgs []llm.Message) llm.CompletionRequest {
	req := llm.CompletionRequest{
		Messages: msgs,
	}
	if a.config.SystemPrompt != "" {
		req.SystemPrompt = a.config.SystemPrompt
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
	if hr := a.emitHook(PreToolCall{Name: tc.Function.Name, Input: input}); hr.Action == hook.ActionAbort {
		return nil, fmt.Errorf("tool %q aborted by hook: %s", tc.Function.Name, hr.Reason)
	}

	callable, ok := a.config.Tools.Get(tc.Function.Name)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", tc.Function.Name)
	}

	toolCtx := tool.NewContext(ctx)
	toolCtx.ToolUseID = tc.ID

	result, err := callable.Call(toolCtx, input)

	// Emit PostToolCall hook
	a.emitHook(PostToolCall{
		Name:   tc.Function.Name,
		Input:  input,
		Result: result,
		Err:    err,
	})

	return result, err
}

func (a *Agent) emitHook(event hook.Event) hook.Result {
	if a.config.Hooks == nil {
		return hook.Continue()
	}
	return a.config.Hooks.Emit(event)
}

func (a *Agent) contextTooLarge(msgs []llm.Message) bool {
	caps := a.config.Provider.Capabilities()
	if caps.MaxContextTokens <= 0 {
		return false
	}
	tokens := a.config.Provider.CountTokens(msgs)
	return tokens > caps.MaxContextTokens
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
