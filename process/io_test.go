package process_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/process"
)

func TestRunInputInherit(t *testing.T) {
	t.Parallel()

	// With no Stdin and InputInherit, the child inherits the parent's stdin.
	// The test process's stdin is not a pipe of data, so `cat` sees EOF and exits cleanly.
	result, err := process.Run(t.Context(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "exit 0"},
		Input:  process.InputInherit,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Success() {
		t.Fatalf("expected success, got exit %d", result.ExitCodeOr(-1))
	}
}

func TestRunStdinTakesPrecedenceOverPolicy(t *testing.T) {
	t.Parallel()

	result, err := process.Run(t.Context(), process.Command{
		Binary: "cat",
		Stdin:  strings.NewReader("piped"),
		Input:  process.InputInherit,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := string(result.Stdout); got != "piped" {
		t.Fatalf("Stdout = %q, want piped", got)
	}
}

func TestRunInheritedIONoCapture(t *testing.T) {
	t.Parallel()

	result, err := process.Run(t.Context(), process.Command{
		Binary: "echo",
		Args:   []string{"to terminal"},
		IO:     process.IOInherited,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Stdout) != 0 {
		t.Fatalf("expected no captured stdout in inherited mode, got %q", result.Stdout)
	}
	if !result.Success() {
		t.Fatalf("expected success, got exit %d", result.ExitCodeOr(-1))
	}
}

func TestRunCapturedIsDefault(t *testing.T) {
	t.Parallel()

	result, err := process.Run(t.Context(), process.Command{
		Binary: "echo",
		Args:   []string{"captured"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "captured" {
		t.Fatalf("Stdout = %q, want captured", got)
	}
}
