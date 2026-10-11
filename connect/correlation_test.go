package connect

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
)

func TestNormalizationCorrelatesWithoutLoggingSecrets(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx := observability.Correlate(trace.ContextWithSpanContext(t.Context(), sc))
	original := apperrors.Internal(errors.New("private-error-sentinel")).WithReason("STORAGE_DOWN")
	call := NormalizingInterceptor(log).WrapUnary(unaryHandler(nil, original))
	_, gotErr := LoggingInterceptor(log)(call)(ctx, newProtoRequest())
	decoded, err := DecodeError(gotErr)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.TraceID != sc.TraceID().String() || decoded.Reason != "STORAGE_DOWN" {
		t.Errorf("normalization lost trace or reason: %+v", decoded)
	}
	if original.TraceID != "" {
		t.Error("normalization mutated the shared application error")
	}
	if strings.Contains(output.String(), "private-error-sentinel") {
		t.Error("private error entered logs")
	}
	if !strings.Contains(output.String(), sc.TraceID().String()) || !strings.Contains(output.String(), sc.SpanID().String()) {
		t.Error("logs lack trace/span correlation")
	}
}
