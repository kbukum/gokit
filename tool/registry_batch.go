package tool

import (
	"encoding/json"
	"fmt"
	"sync"
)

// BatchCall represents a single tool invocation in a batch.
type BatchCall struct {
	Name  string          `json:"name"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

// BatchResult pairs a batch call with its result.
type BatchResult struct {
	ID     string  `json:"id"`
	Result *Result `json:"result,omitempty"`
	Err    error   `json:"error,omitempty"`
}

// BatchOptions controls passive batch execution. The caller owns policy: agent supplies tool concurrency and fail-fast behavior; Registry does not infer concurrency from ReadOnly.
type BatchOptions struct {
	Concurrency int
	FailFast    bool
}

// CallBatch executes multiple tool calls with a caller-supplied concurrency cap.
// Results are returned in the same order as calls.
func (r *Registry) CallBatch(ctx *Context, calls []BatchCall, opts BatchOptions) []BatchResult {
	results := make([]BatchResult, len(calls))
	if len(calls) == 0 {
		return results
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(calls) {
		concurrency = len(calls)
	}
	sem := make(chan struct{}, concurrency)
	done := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	stop := func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	for i, c := range calls {
		if stop() {
			results[i] = BatchResult{ID: c.ID, Err: fmt.Errorf("tool: batch stopped after fail-fast")}
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, c BatchCall) {
			defer wg.Done()
			defer func() { <-sem }()
			if stop() {
				results[i] = BatchResult{ID: c.ID, Err: fmt.Errorf("tool: batch stopped after fail-fast")}
				return
			}
			callCtx := ctx.clone()
			callCtx.ToolUseID = c.ID
			res, err := r.Call(callCtx, c.Name, c.Input)
			results[i] = BatchResult{ID: c.ID, Result: res, Err: err}
			if opts.FailFast && err != nil {
				once.Do(func() { close(done) })
			}
		}(i, c)
	}
	wg.Wait()
	return results
}
