package process_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestRunScrubEnv(t *testing.T) {
	t.Setenv("GOKIT_PROCESS_SECRET", "hidden")

	result, err := process.Run(context.Background(), process.Command{
		Binary:   "sh",
		Args:     []string{"-c", "echo ${GOKIT_PROCESS_SECRET:-missing}:${EXPLICIT_ONLY:-unset}"},
		Env:      []string{"EXPLICIT_ONLY=visible"},
		ScrubEnv: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "missing:visible" {
		t.Fatalf("stdout = %q, want missing:visible", got)
	}
}

func TestRunMaxOutputBytes(t *testing.T) {
	result, err := process.Run(context.Background(), process.Command{
		Binary:         "sh",
		Args:           []string{"-c", "printf '1234567890' && printf 'abcdefghij' >&2"},
		MaxOutputBytes: 4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(result.Stdout); got != "1234" {
		t.Fatalf("stdout = %q, want 1234", got)
	}
	if !result.StdoutTruncated {
		t.Fatal("expected stdout truncation")
	}
	if got := string(result.Stderr); got != "abcd" {
		t.Fatalf("stderr = %q, want abcd", got)
	}
	if !result.StderrTruncated {
		t.Fatal("expected stderr truncation")
	}
}

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
	if runtime.GOOS == "windows" {
		// Windows command-line is capped at ~32 KiB; this test exercises a
		// 100 KB argument which is well within Unix limits but not Windows'.
		t.Skip("Windows command-line length limit is too small for this test")
	}
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
		t.Fatal("expected error from already-canceled context")
	}
}

func TestRunSigtermSigkillEscalation(t *testing.T) {
	// Process that traps SIGTERM and ignores it — must be SIGKILLed
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := process.Run(ctx, process.Command{
		Binary: "sh",
		Args: []string{
			"-c",
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

func TestRunNoShellInjection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	sentinel := filepath.Join(tmpDir, "shell-injection-sentinel.txt")

	// Use echo which is universally available on all platforms.
	// The shell-injection payload is passed as a literal argument.
	result, err := process.Run(context.Background(), process.Command{
		Binary: "echo",
		Args:   []string{"$(touch " + sentinel + ")"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// echo should output the literal string, not execute the subcommand.
	expected := "$(touch " + sentinel + ")\n"
	if got := string(result.Stdout); got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("expected sentinel to be absent (shell injection occurred), stat err=%v", err)
	}
}
