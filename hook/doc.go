// Package hook provides a lightweight event system for agentic lifecycle hooks.
//
// It allows registering handlers for events like PreToolCall, PostToolCall,
// PreLLMCall, PostLLMCall, OnError, TurnStart, and TurnEnd. Handlers run
// sequentially in registration order, with short-circuit on Abort and
// chained Modify results.
//
// Usage:
//
//	registry := hook.NewRegistry()
//	registry.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
//	    pre := e.(hook.PreToolCall)
//	    log.Printf("calling tool: %s", pre.Name)
//	    return hook.Continue()
//	})
package hook
