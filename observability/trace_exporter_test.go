package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeTraceExporterPreservesCauses(t *testing.T) {
	t.Parallel()
	cause := errors.New("synthetic-private-shutdown")
	underlying := &ownedTestExporter{block: true, stopErr: cause}
	exporter := &safeTraceExporter{SpanExporter: underlying}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := exporter.ExportSpans(ctx, nil)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-") {
		t.Fatal("export error lost its cause or exposed private diagnostics")
	}
	err = exporter.Shutdown(t.Context())
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "private-") {
		t.Fatal("shutdown error lost its cause or exposed private diagnostics")
	}
}

func TestSafeTraceExporterSuccess(t *testing.T) {
	t.Parallel()
	exporter := &safeTraceExporter{SpanExporter: &ownedTestExporter{}}
	if err := exporter.ExportSpans(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
