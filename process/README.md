# gokit/process

Subprocess execution with context cancellation, signal handling, and provider integration.

## Overview

The `process` package provides a structured way to run external commands from Go services.
It wraps `os/exec` with context-aware cancellation, process group management,
graceful shutdown (SIGTERM → SIGKILL), and automatic output capture. Results include stdout, stderr,
exit code, and duration.

For long-running or unreliable subprocesses, the package integrates with gokit's provider
and resilience frameworks — adding retry, circuit breaker, and generic I/O adapters.

## Installation

```bash
go get github.com/kbukum/gokit
```

> `process` is part of the core module — no separate `go get` needed.

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/process"
)

func main() {
	ctx := context.Background()

	result, err := process.Run(ctx, process.Command{
		Binary: "echo",
		Args:   []string{"hello", "world"},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(string(result.Stdout)) // "hello world\n"
	fmt.Println(result.ExitCode)       // 0
	fmt.Println(result.Duration)       // e.g. 2.1ms
}
```

## API Reference

### Types

| Type | Description |
|------|-------------|
| `Command` | Subprocess configuration: binary, args, dir, env, stdin, grace period |
| `Result` | Execution output: stdout, stderr, exit code, duration |
| `Config` | Adapter-level defaults for name, grace period, and timeout |
| `Adapter` | Wraps `Run` as a `provider.RequestResponse[Command, *Result]` |
| `Runner` | Wraps `Run` with persistent resilience state (circuit breaker, retry) |
| `SubprocessProvider[I, O]` | Generic provider that builds a command from input and parses output |

### Core Function

```go
func Run(ctx context.Context, cmd Command) (*Result, error)
```

Executes the command, captures output, and returns a `*Result` (always populated, even on error).

## Advanced Usage

### Context Cancellation

When the context is cancelled, `Run` sends SIGTERM to the entire process group.
If the process doesn't exit within `GracePeriod` (default 5s), it escalates to SIGKILL.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

result, err := process.Run(ctx, process.Command{
	Binary:      "long-running-task",
	GracePeriod: 2 * time.Second,
})
// err wraps context.DeadlineExceeded; result.Stdout has partial output
```

### Environment and Working Directory

```go
result, _ := process.Run(ctx, process.Command{
	Binary: "python",
	Args:   []string{"script.py"},
	Dir:    "/opt/scripts",
	Env:    []string{"MODEL=large", "GPU=true"},
})
```

Extra environment variables are merged with the parent process environment.

### Resilient Execution

Use `Runner` for subprocesses that may fail transiently.
Circuit breaker state persists across calls.

```go
runner := process.NewRunner(provider.ResilienceConfig{
	CircuitBreaker: &resilience.CircuitBreakerConfig{
		Threshold: 3, ResetTimeout: 30 * time.Second,
	},
})

result, err := runner.Run(ctx, process.Command{Binary: "flaky-tool"})
```

### Generic Provider

Convert any command-line tool into a typed provider:

```go
p := process.NewSubprocessProvider[string, []Segment](
	"diarizer",
	func(audioPath string) process.Command {
		return process.Command{Binary: "python", Args: []string{"diarize.py", audioPath}}
	},
	func(result *process.Result) ([]Segment, error) {
		var segments []Segment
		return segments, json.Unmarshal(result.Stdout, &segments)
	},
)

segments, err := p.Execute(ctx, "audio.wav")
```

## Testing

```bash
cd process
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
