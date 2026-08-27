package logging

import (
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
)

func TestMapSeverity(t *testing.T) {
	t.Parallel()

	cases := map[slog.Level]otellog.Severity{
		LevelTrace:      otellog.SeverityTrace,
		slog.LevelDebug: otellog.SeverityDebug,
		slog.LevelInfo:  otellog.SeverityInfo,
		slog.LevelWarn:  otellog.SeverityWarn,
		slog.LevelError: otellog.SeverityError,
		LevelFatal:      otellog.SeverityFatal,
	}
	for level, want := range cases {
		if got := mapSeverity(level); got != want {
			t.Errorf("mapSeverity(%v) = %v, want %v", level, got, want)
		}
	}
}

func TestToOTLPKeyValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value any
		want  attribute.Value
	}{
		{"s", attribute.StringValue("s")},
		{true, attribute.BoolValue(true)},
		{7, attribute.IntValue(7)},
		{int64(8), attribute.Int64Value(8)},
		{3.5, attribute.Float64Value(3.5)},
		{[]string{"x"}, attribute.StringValue("[x]")}, // fallback stringifies
	}
	for _, c := range cases {
		got := toOTLPKeyValue("k", c.value)
		if got.Key != "k" {
			t.Errorf("key = %v, want k", got.Key)
		}
		if got.Value != c.want {
			t.Errorf("toOTLPKeyValue(%v) = %v, want %v", c.value, got.Value, c.want)
		}
	}
}

func TestOTLPProviderShutdownNilSafe(t *testing.T) {
	t.Parallel()

	var p *OTLPProvider
	if err := p.Shutdown(t.Context()); err != nil {
		t.Errorf("nil provider Shutdown should be no-op, got %v", err)
	}
}
