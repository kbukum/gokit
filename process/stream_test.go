package process_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func TestStreamEmitsStdoutAndStderr(t *testing.T) {
	var chunks []process.StreamChunk
	result, err := process.Stream(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "printf 'out'; printf 'err' >&2"},
	}, func(chunk process.StreamChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := string(result.Stdout); got != "out" {
		t.Fatalf("Stdout = %q want out", got)
	}
	if got := string(result.Stderr); got != "err" {
		t.Fatalf("Stderr = %q want err", got)
	}

	seen := map[process.StreamName]string{}
	for _, chunk := range chunks {
		seen[chunk.Stream] += string(chunk.Data)
	}
	if seen[process.StreamStdout] != "out" {
		t.Fatalf("stdout chunks = %q want out", seen[process.StreamStdout])
	}
	if seen[process.StreamStderr] != "err" {
		t.Fatalf("stderr chunks = %q want err", seen[process.StreamStderr])
	}
}

func TestStreamMaxOutputBytesTruncatesCapture(t *testing.T) {
	var emitted strings.Builder
	result, err := process.Stream(context.Background(), process.Command{
		Binary:         "sh",
		Args:           []string{"-c", "printf '1234567890'"},
		MaxOutputBytes: 4,
	}, func(chunk process.StreamChunk) {
		emitted.Write(chunk.Data)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := string(result.Stdout); got != "1234" {
		t.Fatalf("Stdout = %q want 1234", got)
	}
	if !result.StdoutTruncated {
		t.Fatal("expected stdout truncation")
	}
	if got := emitted.String(); got != "1234567890" {
		t.Fatalf("emitted = %q want full stream", got)
	}
}

func TestStreamNonzeroExitReturnsResult(t *testing.T) {
	result, err := process.Stream(context.Background(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "printf 'before-fail'; exit 7"},
	}, nil)
	if err == nil {
		t.Fatal("expected nonzero exit error")
	}
	if result == nil {
		t.Fatal("expected result with nonzero exit")
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d want 7", result.ExitCode)
	}
	if got := string(result.Stdout); got != "before-fail" {
		t.Fatalf("Stdout = %q want before-fail", got)
	}
}

func TestStreamContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := process.Stream(ctx, process.Command{
		Binary:      "sleep",
		Args:        []string{"5"},
		GracePeriod: 100 * time.Millisecond,
	}, nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if result == nil {
		t.Fatal("expected cancellation result")
	}
	if result.Duration > 2*time.Second {
		t.Fatalf("Duration = %v, process was not canceled promptly", result.Duration)
	}
}
