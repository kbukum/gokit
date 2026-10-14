# gokit/process

Subprocess execution with context cancellation, signal handling, and provider integration.

## Overview

The `process` package provides argv-only execution with context cancellation, process groups, and structured results. Results distinguish exit status, deadline, cancellation, and forced termination. Set a positive `Command.MaxOutputBytes` for bounded Run/Stream capture; zero means unlimited.

For long-running or unreliable subprocesses, the package integrates with gokit's provider and resilience frameworks — adding retry, circuit breaker, and generic I/O adapters.

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
	fmt.Println(result.ExitCodeOr(-1)) // 0 on success; -1 if the process was killed
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

### Owned test processes

`process/testutil.BuildGo` builds child programs with explicit race and coverage settings; parent test flags do not instrument children automatically. Set `GoBuild.Race` to `testutil.RaceEnabled` to follow the owning executable's build mode, or to `true` for a deliberately instrumented child. Coverage remains independently selected through `GoBuild.Coverage` and `CoverPackages`; the caller owns the binary and counter directory. A plain pass is not race evidence.

`StartPersistent` owns a long-lived child until you call `Shutdown`. Startup readiness uses an output marker or an explicit stabilization delay; immediate readiness certifies only spawn. Configure the child with an isolated working directory, `EnvEmpty`, explicit environment values, and a positive output limit. Lifetime capture defaults to 64 KiB per stream and reports truncation.

Emit the readiness marker only after the child has installed shutdown handling and initialized any required descendants. A shell printing a marker before starting another program does not establish that program's readiness; immediate shutdown can race its creation and require forced cleanup.

```go
cfg := process.DefaultPersistentConfig()
cfg.Readiness = process.ReadyOnOutput
cfg.OutputMarker = "READY"
cfg.ReadinessTimeout = 30 * time.Second
run, err := process.StartPersistent(t.Context(), command, cfg)
if run != nil {
    t.Cleanup(func() {
        cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
        defer cancel()
        outcome, cleanupErr := run.Process.Shutdown(cleanupCtx)
        if cleanupErr != nil {
            t.Error(cleanupErr)
        }
        if !outcome.Complete {
            t.Error("cleanup incomplete: retain the run, resolve the blocker, and retry Shutdown")
        }
    })
}
if err != nil {
    t.Fatal(err)
}
// Exercise the ready child; cleanup is installed even for a failed-start lease.
```

Shutdown sends SIGTERM on Linux and Darwin, waits the configured grace period, escalates if required, and reaps the child. The original child stays unreaped until its isolated group is released, so its PID/PGID cannot be reused during cleanup. A parent exiting zero does not release a retained descendant. Group-signal helpers are for commands still owned and unreaped; a diagnostic PID is never permission to signal a later process.

A forced shutdown returns both `Result.Forced` and an error. Deadline/cancellation remain separately observable. `ShutdownOutcome.Complete` confirms group release and completed Wait/output collection; a false value retains cleanup ownership for retry. Historical failures remain inspectable even after a retry completes. Repeated completed shutdown returns the recorded result without signaling again. Graceful waiting has a 10-second ceiling and forced settling gets a fresh two-second ceiling. Native inspection failures are surfaced, with at most eight failed inspections per cleanup wait rather than a permanently disabled cleanup path.

On startup failure, `StartPersistent` normally releases the acquisition and returns nil with the classified error. If cleanup itself is incomplete, it returns an error **and a non-nil owning run**. Keep that handle and retry cleanup after resolving the blocker; it does not certify readiness.

`Wait(ctx)` returns `(*Result, error)`. It observes exit without stopping a live child; after exit it drains the retained group and reaps before returning the completed result. Canceling a wait does not stop a still-live child. Native inspection failure is an error, not proof of exit. A spontaneous nonzero exit is an error, not an automatic restart.

`WaitExit(ctx)` performs the same owned drain/reap but leaves exit-status assertions to the caller: a deliberate crash or nonzero exit is recorded in `Result`, while observation, capture and unexpected forced cleanup still fail. A result is returned only after complete cleanup. `Signal` targets only the retained unreaped child, refuses exited/released identities and never uses a historical PID. Optional `PersistentConfig.Stdout`/`Stderr` writers observe live output; they must be prompt and retention-bounded. Writer failures are retained, subsequent calls to that writer stop, and pipes continue draining.

`Supervisor.Shutdown(ctx, reason)` returns `(ShutdownReport, error)` with per-child results, completion, and reaping status. Completed children and their errors remain in the report across partial-cleanup retries. `Track` transfers exclusive reaping of an unreaped command; configure its group before Start when using descendant termination. Repeated registration of the same command returns its existing handle. `Release` transfers ownership back only before cleanup starts and returns an error if it would abandon pending cleanup. `TrackPid` is explicitly best-effort and signal-only: it cannot verify process identity or reap a child.

Owned persistent/supervised commands support Linux (with `/proc` available), Darwin, and Windows. Linux stat reads are capped at 4 KiB and group scans use 128-entry batches. Windows uses its managed process handle, reports owned termination as forced, and does not support the Unix group or bare-PID contract. Other platforms fail explicitly for this owned-observation path. Go cannot resolve arbitrary kernel stalls or force arbitrary caller-provided I/O callbacks to return; incomplete cleanup stays owned and observable.

### Context Cancellation

When the context is cancelled, `Run` sends SIGTERM to the entire process group. If the process doesn't exit within `GracePeriod` (default 5s), it escalates to SIGKILL.

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
	Env:    map[string]string{"MODEL": "large", "GPU": "true"},
})
```

Env entries are merged onto the base environment selected by `EnvPolicy` (`EnvInherit` by default, or `EnvEmpty` to start from an empty environment).

### Resilient Execution

Use `Runner` for subprocesses that may fail transiently. Circuit breaker state persists across calls.

```go
runner, err := process.NewRunner(provider.ResilienceConfig{
	CircuitBreaker: &resilience.CircuitBreakerConfig{
		MaxFailures: 3, Timeout: 30 * time.Second,
	},
})
if err != nil {
	return err
}

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

`process/testutil.BuildGo` builds one child program with explicit race/atomic-coverage instrumentation, independent of the parent test flags. It fixes `GOWORK=off`, bounds the build to five minutes and captures at most 1 MiB per diagnostic stream; failure and overflow preserve their causes. The caller owns the output path, `GOCOVERDIR`, artifact reuse and compatible coverage merging.

```bash
cd process
go test -race ./...
```

## Contributing

Please refer to the root [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
