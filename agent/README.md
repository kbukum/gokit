# gokit/agent

`agent` owns the bounded LLM/tool loop. Defaults are bounded: `MaxTurns=10`, `WallClock=60s`, `MaxTokens=100000`, `MaxToolCalls=50`, `ToolConcurrency=4`, and `ToolTimeout=30s`.

## Install

```bash
go get github.com/kbukum/gokit/agent
go get github.com/kbukum/gokit/llm
go get github.com/kbukum/gokit/llm/providers/ollama
```

## Architecture

```mermaid
flowchart TD
    Agent[agent]
    Loop[Run loop\nturn budget memory]
    Hooks[hook callbacks]
    Commands[command handling]
    Memory[memory policy]
    LLM[llm.Provider]
    Tools[tool.Registry]
    Skill[skill.Registry]
    MCP[mcp callables]
    Authz[authz.Decider]

    Agent --> Loop
    Agent --> Hooks
    Agent --> Commands
    Agent --> Memory
    Loop --> LLM
    Loop --> Tools
    Loop --> Skill
    Loop --> MCP
    Loop --> Authz
```

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/ollama"
)

func main() {
	registry := llm.NewDialectRegistry()
	if err := ollama.Register(registry); err != nil {
		panic(err)
	}

	adapter, err := llm.New(registry, llm.Config{
		Dialect: ollama.DialectName,
		BaseURL: ollama.DefaultBaseURL,
		Model:   "llama3.2",
	})
	if err != nil {
		panic(err)
	}

	provider := llm.NewProvider(adapter, "llama3.2")
	runner := agent.New(agent.Config{
		Provider:     provider,
		Model:        "llama3.2",
		SystemPrompt: "You are concise and operationally precise.",
	})

	result, err := runner.Run(context.Background(), []chat.Message{
		chat.User("Write a two-line release summary."),
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(result.FinalMessage.Text())
}
```

## Streaming

`Run` returns the final `Result`. `Stream` drives the same bounded loop and emits the turn lifecycle as a channel of `AgentEvent` values — `TurnStart`, `LLMDelta` (each provider token/tool-use delta), `ToolExecuting`/`ToolComplete`, `Compacted`, `TurnComplete`, and a terminal `RunComplete` carrying the final `Result`. The channel is bounded by `Config.StreamBuffer`; a canceled context or a stalled consumer stops the loop and closes the channel.

```go
events, err := runner.Stream(ctx, []chat.Message{chat.User("summarize the release")})
if err != nil {
	panic(err)
}
for evt := range events {
	switch e := evt.(type) {
	case agent.LLMDelta:
		if td, ok := e.Event.(llm.TextDelta); ok {
			fmt.Print(td.Text)
		}
	case agent.ToolExecuting:
		fmt.Printf("\n[calling %s]\n", e.Name)
	case agent.RunComplete:
		if e.Err != nil {
			panic(e.Err)
		}
		fmt.Println("\nstop:", e.Result.StopReason)
	}
}
```

## When to use

Use `agent` when you want the bounded turn loop, budgets, tool dispatch, hooks, and memory policy in one place instead of building an orchestration loop yourself.
