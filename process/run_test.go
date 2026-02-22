package process_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func TestRunEcho(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	out := strings.TrimSpace(string(result.Stdout))
	if out != "hello world" {
		t.Fatalf("expected 'hello world', got %q", out)
	}
}

func TestRunStdin(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "cat",
		Stdin:  strings.NewReader("from stdin"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := string(result.Stdout)
	if out != "from stdin" {
		t.Fatalf("expected 'from stdin', got %q", out)
	}
}

func TestRunExitCode(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "exit 42"},
	})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestRunStderr(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "echo oops >&2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stderr := strings.TrimSpace(string(result.Stderr))
	if stderr != "oops" {
		t.Fatalf("expected 'oops' on stderr, got %q", stderr)
	}
}

func TestRunContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := process.Run(ctx, process.Command{
		Binary:      "sleep",
		Args:        []string{"10"},
		GracePeriod: 500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if result.Duration > 5*time.Second {
		t.Fatalf("process took too long to kill: %v", result.Duration)
	}
}

func TestRunEmptyBinary(t *testing.T) {
	_, err := process.Run(context.Background(), process.Command{})
	if err == nil {
		t.Fatal("expected error for empty binary")
	}
}

func TestRunDuration(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sleep",
		Args:   []string{"0.1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Duration < 50*time.Millisecond {
		t.Fatalf("duration too short: %v", result.Duration)
	}
}

func TestRunEnv(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "echo $MY_TEST_VAR"},
		Env:    []string{"MY_TEST_VAR=hello123"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(string(result.Stdout))
	if out != "hello123" {
		t.Fatalf("expected 'hello123', got %q", out)
	}
}
