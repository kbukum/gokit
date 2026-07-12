package worker_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/worker"
)

func TestEventTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ  worker.EventType
		want string
	}{
		{worker.EventProgress, "progress"},
		{worker.EventPartial, "partial"},
		{worker.EventLog, "log"},
		{worker.EventResult, "result"},
		{worker.EventError, "error"},
	}

	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestProgressEvent(t *testing.T) {
	t.Parallel()

	e := worker.ProgressEvent[string](50, 100, "halfway")
	if e.Type != worker.EventProgress {
		t.Fatalf("expected EventProgress, got %v", e.Type)
	}
	if e.Progress == nil {
		t.Fatal("expected non-nil progress")
	}
	if e.Progress.Current != 50 {
		t.Fatalf("expected current=50, got %d", e.Progress.Current)
	}
	if e.Progress.Total != 100 {
		t.Fatalf("expected total=100, got %d", e.Progress.Total)
	}
	if e.Progress.Percent != 0.5 {
		t.Fatalf("expected percent=0.5, got %f", e.Progress.Percent)
	}
	if e.Progress.Message != "halfway" {
		t.Fatalf("expected message 'halfway', got %q", e.Progress.Message)
	}
}

func TestPartialEvent(t *testing.T) {
	t.Parallel()

	e := worker.PartialEvent("data")
	if e.Type != worker.EventPartial {
		t.Fatalf("expected EventPartial, got %v", e.Type)
	}
	if e.Data != "data" {
		t.Fatalf("expected data 'data', got %q", e.Data)
	}
}

func TestLogEvent(t *testing.T) {
	t.Parallel()

	e := worker.LogEvent[string]("test message", map[string]any{"key": "val"})
	if e.Type != worker.EventLog {
		t.Fatalf("expected EventLog, got %v", e.Type)
	}
	if e.Metadata["message"] != "test message" {
		t.Fatalf("expected message in metadata, got %v", e.Metadata)
	}
	if e.Metadata["key"] != "val" {
		t.Fatalf("expected key=val in metadata, got %v", e.Metadata)
	}
}

func TestLogEventNilMeta(t *testing.T) {
	t.Parallel()

	e := worker.LogEvent[string]("no meta", nil)
	if e.Type != worker.EventLog {
		t.Fatalf("expected EventLog, got %v", e.Type)
	}
	if e.Metadata == nil {
		t.Fatal("expected non-nil metadata even when nil passed")
	}
	if e.Metadata["message"] != "no meta" {
		t.Fatalf("expected message in metadata, got %v", e.Metadata)
	}
}

func TestEventTypeStringUnknown(t *testing.T) {
	t.Parallel()

	unknown := worker.EventType(99)
	s := unknown.String()
	if s != "unknown" {
		t.Fatalf("expected 'unknown', got %q", s)
	}
}

func TestProgressEventZeroTotal(t *testing.T) {
	t.Parallel()

	e := worker.ProgressEvent[string](50, 0, "unknown")
	if e.Progress.Percent != 0 {
		t.Fatalf("expected percent=0 when total=0, got %f", e.Progress.Percent)
	}
}

func TestEventOrdering(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		emit(worker.ProgressEvent[int](1, 3, "step 1"))
		emit(worker.ProgressEvent[int](2, 3, "step 2"))
		emit(worker.PartialEvent(task * 2))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "order-test", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), 5)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	types := make([]worker.EventType, 0, 4)
	for e := range handle.Events() {
		types = append(types, e.Type)
	}

	// Expected: Progress, Progress, Partial, Result
	expected := []worker.EventType{
		worker.EventProgress,
		worker.EventProgress,
		worker.EventPartial,
		worker.EventResult,
	}

	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i := range expected {
		if types[i] != expected[i] {
			t.Errorf("event[%d]: expected %s, got %s", i, expected[i], types[i])
		}
	}
}
