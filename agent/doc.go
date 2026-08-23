// Package agent provides the orchestration loop that composes LLM providers, tool registries,
// and hook registries into an agentic conversation loop.
//
// The agent iteratively calls the LLM, executes tool calls from the response, feeds results back,
// and repeats until the model stops requesting tools.
//
// [Agent.Run] returns the final [Result]. [Agent.Stream] drives the same bounded loop but emits
// the full turn lifecycle as a stream of [AgentEvent] values (turn boundaries, provider token
// deltas, tool execution, context compaction, and a terminal [RunComplete]) over a bounded
// channel, so an interactive caller need not re-run the loop.
//
// Usage:
//
//	a := agent.New(agent.Config{
//	    Provider:     myProvider,
//	    Tools:        myRegistry,
//	    SystemPrompt: "You are a helpful assistant.",
//	})
//	result, err := a.Run(ctx, []llm.Message{llm.User("hello")})
package agent
