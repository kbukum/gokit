package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/worker"
)

// TestProgressEventGoldenJSON locks the shared wire shape: percent on a 0–100 scale,
// task_id as a string UUID, and a known total serialized as a number.
func TestProgressEventGoldenJSON(t *testing.T) {
	t.Parallel()

	e := worker.ProgressEvent[string](50, 100, "halfway")
	e.TaskID = "8b6cf1e2-0e0a-4f5b-9c2d-1a2b3c4d5e6f"
	e.WorkerID = "w1"

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["type"] != "progress" {
		t.Fatalf("type = %v, want \"progress\" (string discriminant)", decoded["type"])
	}

	if _, err := uuid.Parse(decoded["task_id"].(string)); err != nil {
		t.Fatalf("task_id is not a string UUID: %v (%v)", decoded["task_id"], err)
	}

	prog, ok := decoded["progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress not an object: %v", decoded["progress"])
	}
	if prog["percent"] != float64(50) {
		t.Fatalf("percent = %v, want 50 (0–100 scale)", prog["percent"])
	}
	if prog["total"] != float64(100) {
		t.Fatalf("total = %v, want 100", prog["total"])
	}
	if prog["current"] != float64(50) {
		t.Fatalf("current = %v, want 50", prog["current"])
	}
}

// TestProgressUnknownTotalGoldenJSON locks the unknown-total wire shape: total and percent
// are both omitted (matches rskit's skip-when-none encoding).
func TestProgressUnknownTotalGoldenJSON(t *testing.T) {
	t.Parallel()

	e := worker.ProgressEvent[string](7, -1, "streaming")
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	prog := decoded["progress"].(map[string]any)

	if _, present := prog["total"]; present {
		t.Fatalf("expected total omitted for unknown total, got %v", prog["total"])
	}
	if _, present := prog["percent"]; present {
		t.Fatalf("expected percent omitted for unknown total, got %v", prog["percent"])
	}
}

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

// TestEventTypeJSONRoundTrip locks the snake_case string discriminant on the wire and its
// decode back into the typed enum.
func TestEventTypeJSONRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		typ  worker.EventType
		wire string
	}{
		{worker.EventProgress, `"progress"`},
		{worker.EventPartial, `"partial"`},
		{worker.EventLog, `"log"`},
		{worker.EventResult, `"result"`},
		{worker.EventError, `"error"`},
	} {
		raw, err := json.Marshal(tt.typ)
		if err != nil {
			t.Fatalf("marshal %v: %v", tt.typ, err)
		}
		if string(raw) != tt.wire {
			t.Errorf("marshal %v = %s, want %s", tt.typ, raw, tt.wire)
		}
		var got worker.EventType
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if got != tt.typ {
			t.Errorf("round-trip = %v, want %v", got, tt.typ)
		}
	}

	if err := json.Unmarshal([]byte(`"bogus"`), new(worker.EventType)); err == nil {
		t.Error("expected error decoding unknown event type")
	}
}

// TestErrorEventJSONRoundTrip confirms an error event serializes its error as a wire string
// and decodes back into a non-nil error, so error events survive a cross-kit round trip
// instead of collapsing to an empty object.
func TestErrorEventJSONRoundTrip(t *testing.T) {
	t.Parallel()

	e := worker.Event[string]{
		Type:     worker.EventError,
		TaskID:   "t1",
		WorkerID: "w1",
		Error:    errors.New("boom"),
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if wire["error"] != "boom" {
		t.Errorf("wire error = %v, want %q", wire["error"], "boom")
	}

	var decoded worker.Event[string]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != worker.EventError || decoded.TaskID != "t1" || decoded.WorkerID != "w1" {
		t.Errorf("decoded event = %+v", decoded)
	}
	if decoded.Error == nil || decoded.Error.Error() != "boom" {
		t.Errorf("decoded error = %v, want %q", decoded.Error, "boom")
	}
}

// TestEventWithoutErrorOmitsField confirms an error-free event omits the error field and
// decodes with a nil error.
func TestEventWithoutErrorOmitsField(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(worker.PartialEvent[string]("chunk"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"error"`) {
		t.Errorf("expected no error field, got %s", raw)
	}

	var decoded worker.Event[string]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Error != nil {
		t.Errorf("decoded error = %v, want nil", decoded.Error)
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
	if e.Progress.Total == nil || *e.Progress.Total != 100 {
		t.Fatalf("expected total=100, got %v", e.Progress.Total)
	}
	if e.Progress.Percent == nil || *e.Progress.Percent != 50 {
		t.Fatalf("expected percent=50 (0–100 scale), got %v", e.Progress.Percent)
	}
	if e.Progress.Message != "halfway" {
		t.Fatalf("expected message 'halfway', got %q", e.Progress.Message)
	}
}

func TestProgressUnknownTotal(t *testing.T) {
	t.Parallel()

	e := worker.ProgressEvent[string](7, -1, "streaming")
	if e.Progress.Total != nil {
		t.Fatalf("expected nil total for unknown, got %v", *e.Progress.Total)
	}
	if e.Progress.Percent != nil {
		t.Fatalf("expected nil percent for unknown total, got %v", *e.Progress.Percent)
	}
}

func TestNewProgressZeroTotalIsComplete(t *testing.T) {
	t.Parallel()

	total := int64(0)
	p := worker.NewProgress(0, &total)
	if p.Percent == nil || *p.Percent != 100 {
		t.Fatalf("expected percent=100 for zero total, got %v", p.Percent)
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

	e := worker.ProgressEvent[string](0, 0, "empty")
	if e.Progress.Percent == nil || *e.Progress.Percent != 100 {
		t.Fatalf("expected percent=100 when total=0 (nothing to do is complete), got %v", e.Progress.Percent)
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
