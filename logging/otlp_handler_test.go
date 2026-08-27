package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// memExporter is a real synchronous sdklog.Exporter that records emitted
// records, letting tests assert what the OTLP sink produced.
type memExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}
func (e *memExporter) Shutdown(context.Context) error   { return nil }
func (e *memExporter) ForceFlush(context.Context) error { return nil }

func newMemProvider(exp *memExporter) *OTLPProvider {
	p := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	return &OTLPProvider{provider: p, logger: p.Logger(otlpLoggerName)}
}

func TestOTLPHandlerEmitsRecord(t *testing.T) {
	t.Parallel()

	exp := &memExporter{}
	provider := newMemProvider(exp)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	lv := new(slog.LevelVar)
	lv.Set(slog.LevelInfo)
	h := newOTLPHandler(provider, lv).WithAttrs([]slog.Attr{slog.String("bound", "b")})

	rec := slog.NewRecord(time.Now(), slog.LevelError, "kaboom", 0)
	rec.AddAttrs(slog.String("k", "v"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	exp.mu.Lock()
	defer exp.mu.Unlock()
	if len(exp.records) != 1 {
		t.Fatalf("expected 1 exported record, got %d", len(exp.records))
	}
	got := &exp.records[0]
	if got.Severity() != otellog.SeverityError {
		t.Errorf("severity = %v, want error", got.Severity())
	}
	if got.Body().AsString() != "kaboom" {
		t.Errorf("body = %q, want kaboom", got.Body().AsString())
	}
	attrs := map[string]string{}
	got.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value.AsString()
		return true
	})
	if attrs["bound"] != "b" || attrs["k"] != "v" {
		t.Errorf("attributes = %v, want bound=b k=v", attrs)
	}
}

func TestOTLPHandlerEnabledGating(t *testing.T) {
	t.Parallel()

	lv := new(slog.LevelVar)
	lv.Set(slog.LevelWarn)
	h := newOTLPHandler(newMemProvider(&memExporter{}), lv)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info should be gated out at warn")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("error should be enabled at warn")
	}

	var nilProvider *OTLPProvider
	if newOTLPHandler(nilProvider, lv).Enabled(context.Background(), slog.LevelError) {
		t.Error("nil provider handler should report disabled")
	}
}

func TestOTLPHandlerQualifiesGroupedAttrs(t *testing.T) {
	t.Parallel()

	exp := &memExporter{}
	provider := newMemProvider(exp)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	lv := new(slog.LevelVar)
	lv.Set(slog.LevelInfo)

	// A group must prefix both attrs bound under it and per-record attrs, so the
	// OTLP export uses the same dotted keys as the other sinks.
	h := newOTLPHandler(provider, lv).
		WithGroup("db").
		WithAttrs([]slog.Attr{slog.String("host", "h")})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "query", 0)
	rec.AddAttrs(slog.String("table", "users"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	exp.mu.Lock()
	defer exp.mu.Unlock()
	if len(exp.records) != 1 {
		t.Fatalf("expected 1 exported record, got %d", len(exp.records))
	}
	attrs := map[string]string{}
	exp.records[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value.AsString()
		return true
	})
	if attrs["db.host"] != "h" || attrs["db.table"] != "users" {
		t.Errorf("attributes = %v, want db.host=h db.table=users", attrs)
	}
}
