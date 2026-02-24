package process_test

import (
	"context"
	"errors"
	"testing"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

func TestRunWithResilience_EmptyConfig(t *testing.T) {
	result, err := process.RunWithResilience(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if string(result.Stdout) != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", string(result.Stdout))
	}
}

func TestRunWithResilience_RetryOnFailure(t *testing.T) {
	runner := process.NewRunner(provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    2,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  1.0,
		},
	})
	// "false" always fails — should fail after 2 attempts
	_, err := runner.Run(context.Background(), process.Command{
		Binary: "false",
	})
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

func TestRunWithResilience_CircuitBreakerTrips(t *testing.T) {
	runner := process.NewRunner(provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "test-proc-cb",
			MaxFailures:      2,
			Timeout:          time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Fail twice to trip CB
	for i := 0; i < 2; i++ {
		_, err := runner.Run(context.Background(), process.Command{
			Binary: "false",
		})
		if err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}

	// Third call should be rejected by CB — wrapped as AppError
	_, err := runner.Run(context.Background(), process.Command{
		Binary: "false",
	})
	if err == nil {
		t.Fatal("expected circuit breaker to reject")
	}
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != goerrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected SERVICE_UNAVAILABLE code, got %s", appErr.Code)
	}
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen in cause chain, got %v", err)
	}
}

func TestRunWithResilience_SuccessDoesNotTripCB(t *testing.T) {
	runner := process.NewRunner(provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "test-proc-success",
			MaxFailures: 3,
			Timeout:     time.Second,
		},
	})

	for i := 0; i < 5; i++ {
		result, err := runner.Run(context.Background(), process.Command{
			Binary: "echo",
			Args:   []string{"ok"},
		})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("call %d: expected exit 0, got %d", i, result.ExitCode)
		}
	}
}

func TestSubprocessProvider_Execute(t *testing.T) {
	p := process.NewSubprocessProvider[string, string](
		"echo-provider",
		func(input string) process.Command {
			return process.Command{
				Binary: "echo",
				Args:   []string{input},
			}
		},
		func(result *process.Result) (string, error) {
			// Trim trailing newline
			out := string(result.Stdout)
			if out != "" && out[len(out)-1] == '\n' {
				out = out[:len(out)-1]
			}
			return out, nil
		},
	)

	if p.Name() != "echo-provider" {
		t.Fatalf("expected name echo-provider, got %s", p.Name())
	}
	if !p.IsAvailable(context.Background()) {
		t.Fatal("expected provider to be available")
	}

	result, err := p.Execute(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result)
	}
}

func TestSubprocessProvider_WithResilience(t *testing.T) {
	p := process.NewSubprocessProvider[string, string](
		"resilient-echo",
		func(input string) process.Command {
			return process.Command{
				Binary: "echo",
				Args:   []string{input},
			}
		},
		func(result *process.Result) (string, error) {
			out := string(result.Stdout)
			if out != "" && out[len(out)-1] == '\n' {
				out = out[:len(out)-1]
			}
			return out, nil
		},
	)

	// Wrap with resilience
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "echo-cb",
			MaxFailures: 3,
			Timeout:     time.Second,
		},
	})

	result, err := wrapped.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "test" {
		t.Fatalf("expected 'test', got %q", result)
	}
}
