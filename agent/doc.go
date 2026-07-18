// Package agent provides the orchestration loop that composes LLM providers, tool registries, and hook registries into an agentic conversation loop.
//
// The agent iteratively calls the LLM, executes tool calls from the response, feeds results back, and repeats until the model stops requesting tools.
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
