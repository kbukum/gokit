package process_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

func TestRunNonexistentWorkingDir(t *testing.T) {
	_, err := process.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"hi"},
		Dir:    "/nonexistent_dir_xyz_99999",
	})
	if err == nil {
		t.Fatal("expected error for non-existent working directory")
	}
}

func TestRunVeryLongArgs(t *testing.T) {
	longArg := strings.Repeat("x", 100_000)
	result, err := process.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{longArg},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(string(result.Stdout))
	if len(out) != 100_000 {
		t.Fatalf("expected 100000 chars, got %d", len(out))
	}
}

func TestRunNonexistentBinary(t *testing.T) {
	_, err := process.Run(context.Background(), process.Command{
		Binary: "nonexistent_binary_xyz_99999",
	})
	if err == nil {
		t.Fatal("expected error for non-existent binary")
	}
	if !strings.Contains(err.Error(), "exit code") && !strings.Contains(err.Error(), "not found") &&
		!strings.Contains(err.Error(), "executable file not found") && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("expected 'not found' style error, got: %v", err)
	}
}

func TestRunEmptyArgs(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestRunAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := process.Run(ctx, process.Command{
		Binary:      "echo",
		Args:        []string{"should not run"},
		GracePeriod: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error from already-cancelled context")
	}
}

func TestRunSigtermSigkillEscalation(t *testing.T) {
	// Process that traps SIGTERM and ignores it — must be SIGKILLed
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := process.Run(ctx, process.Command{
		Binary: "sh",
		Args: []string{"-c",
			"trap '' TERM; sleep 60",
		},
		GracePeriod: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	// Total should be ~context timeout (200ms) + grace period (300ms) ≈ 500ms
	if elapsed > 5*time.Second {
		t.Fatalf("process took too long to kill (SIGKILL escalation failed): %v", elapsed)
	}
}

func TestRunLargeStdout(t *testing.T) {
	// Generate ~1MB of output
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "dd if=/dev/zero bs=1024 count=1024 2>/dev/null | tr '\\0' 'A'"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Stdout) < 1024*1024 {
		t.Fatalf("expected >= 1MB stdout, got %d bytes", len(result.Stdout))
	}
}

func TestRunLargeStderr(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "dd if=/dev/zero bs=1024 count=512 2>/dev/null | tr '\\0' 'E' >&2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Stderr) < 512*1024 {
		t.Fatalf("expected >= 512KB stderr, got %d bytes", len(result.Stderr))
	}
}

func TestRunConcurrent(t *testing.T) {
	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	results := make([]*process.Result, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = process.Run(context.Background(), process.Command{
				Binary: "echo",
				Args:   []string{"concurrent"},
			})
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if results[i].ExitCode != 0 {
			t.Fatalf("goroutine %d: expected exit 0, got %d", i, results[i].ExitCode)
		}
		out := strings.TrimSpace(string(results[i].Stdout))
		if out != "concurrent" {
			t.Fatalf("goroutine %d: expected 'concurrent', got %q", i, out)
		}
	}
}

func TestRunDurationAccuracy(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sleep",
		Args:   []string{"0.2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration < 150*time.Millisecond {
		t.Fatalf("duration too short: %v (expected ~200ms)", result.Duration)
	}
	if result.Duration > 2*time.Second {
		t.Fatalf("duration too long: %v (expected ~200ms)", result.Duration)
	}
}

func TestRunWithResilience_RetryTransientFailure(t *testing.T) {
	runner := process.NewRunner(provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	// "false" always fails — retries should be exhausted
	_, err := runner.Run(context.Background(), process.Command{
		Binary: "false",
	})
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}

	// A succeeding command should work through the runner
	result, err := runner.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"retry ok"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(result.Stdout), "retry ok") {
		t.Fatalf("expected 'retry ok', got %q", string(result.Stdout))
	}
}

func TestRunWithResilience_CircuitBreakerAfterNFailures(t *testing.T) {
	runner := process.NewRunner(provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "test-cb-n-failures",
			MaxFailures:      3,
			Timeout:          5 * time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Fail 3 times to trip the circuit breaker
	for i := 0; i < 3; i++ {
		_, err := runner.Run(context.Background(), process.Command{
			Binary: "false",
		})
		if err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}

	// Next call should be rejected by circuit breaker (fast-fail, no subprocess)
	start := time.Now()
	_, err := runner.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"should not run"},
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected circuit breaker to reject call")
	}
	// Circuit breaker rejection should be near-instant (< 100ms)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("circuit breaker rejection took too long: %v", elapsed)
	}
}
