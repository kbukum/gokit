//go:build !windows

package worker_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/worker"
)

func TestSubprocessHandler(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{
		Command: process.Command{Binary: "echo"},
	})

	var events []worker.Event[worker.SubprocessOutput]
	emit := func(e worker.Event[worker.SubprocessOutput]) { events = append(events, e) }

	err := h.Handle(context.Background(), worker.SubprocessInput{
		Args: []string{"hello", "world"},
	}, emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	found := false
	for _, e := range events {
		if e.Type == worker.EventPartial && e.Data.Line == "hello world" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected partial event with 'hello world', got %+v", events)
	}
}

func TestSubprocessHandlerStderr(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{
		Command: process.Command{Binary: "sh"},
	})

	var events []worker.Event[worker.SubprocessOutput]
	emit := func(e worker.Event[worker.SubprocessOutput]) { events = append(events, e) }

	err := h.Handle(context.Background(), worker.SubprocessInput{
		Args: []string{"-c", "echo oops >&2"},
	}, emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range events {
		if e.Type == worker.EventPartial && e.Data.Stream == "stderr" && e.Data.Line == "oops" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stderr event with 'oops', got %+v", events)
	}
}

func TestSubprocessHandlerEmptyBinary(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{})

	err := h.Handle(context.Background(), worker.SubprocessInput{
		Args: []string{"test"},
	}, func(worker.Event[worker.SubprocessOutput]) {})
	if err == nil {
		t.Fatal("expected error for empty binary")
	}
}

func TestSubprocessHandlerContextCancel(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{
		Command: process.Command{Binary: "sleep"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := h.Handle(ctx, worker.SubprocessInput{
		Args: []string{"10"},
	}, func(worker.Event[worker.SubprocessOutput]) {})
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestSubprocessHandlerNonZeroExit(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{
		Command: process.Command{Binary: "sh"},
	})

	var events []worker.Event[worker.SubprocessOutput]
	emit := func(e worker.Event[worker.SubprocessOutput]) { events = append(events, e) }

	err := h.Handle(context.Background(), worker.SubprocessInput{
		Args: []string{"-c", "exit 42"},
	}, emit)
	if err == nil {
		t.Fatal("expected error from non-zero exit code")
	}
}

func TestSubprocessHandlerWithEnv(t *testing.T) {
	t.Parallel()

	h := worker.NewSubprocessHandler(worker.SubprocessConfig{
		Command: process.Command{
			Binary: "sh",
			Env:    []string{"MY_VAR=hello_from_env"},
			Dir:    "/tmp",
		},
	})

	var events []worker.Event[worker.SubprocessOutput]
	emit := func(e worker.Event[worker.SubprocessOutput]) { events = append(events, e) }

	err := h.Handle(context.Background(), worker.SubprocessInput{
		Args: []string{"-c", "echo $MY_VAR"},
	}, emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range events {
		if e.Type == worker.EventPartial && e.Data.Line == "hello_from_env" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected partial event with 'hello_from_env', got %+v", events)
	}
}
