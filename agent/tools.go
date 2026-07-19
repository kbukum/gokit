package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/tool"
)

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
