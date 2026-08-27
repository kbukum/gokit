package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// errWriter is a real io.Writer that always fails, used to prove the fanout
// surfaces sink errors instead of swallowing them.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestFanoutDispatchesToEverySink(t *testing.T) {
	t.Parallel()

	var a, b bytes.Buffer
	h := newFanout(slog.NewJSONHandler(&a, nil), slog.NewJSONHandler(&b, nil))
	l := slog.New(h)
	l.Info("shared")

	if !strings.Contains(a.String(), "shared") || !strings.Contains(b.String(), "shared") {
		t.Errorf("both sinks should receive the record: a=%q b=%q", a.String(), b.String())
	}
}

func TestFanoutJoinsSinkErrors(t *testing.T) {
	t.Parallel()

	var ok bytes.Buffer
	h := newFanout(slog.NewJSONHandler(&ok, nil), slog.NewJSONHandler(errWriter{}, nil))

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
	err := h.Handle(context.Background(), rec)
	if err == nil {
		t.Fatal("expected the failing sink's error to surface")
	}
	if !strings.Contains(ok.String(), "msg") {
		t.Error("healthy sink should still receive the record despite the failure")
	}
}

func TestFanoutSingleHandlerIsTransparent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	if got := newFanout(inner, nil); got != inner {
		t.Errorf("single non-nil handler should be returned unwrapped, got %T", got)
	}
}

func TestContextIDsFoldInViaCtxMethods(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTraceID(context.Background(), "trace-9")
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.InfoCtx(ctx, "with-ctx")

	m := decodeLine(t, &buf)
	if m[FieldTraceID] != "trace-9" {
		t.Errorf("trace_id = %v, want trace-9", m[FieldTraceID])
	}
}
