package httpx_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/server/httpx"
)

func TestProblemCorrelationAndLogPrivacy(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	ctx := logging.ContextWithLogger(observability.Correlate(trace.ContextWithSpanContext(t.Context(), sc)), log)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/private-path-sentinel", http.NoBody)
	w := httptest.NewRecorder()
	httpx.WriteProblemDetails(w, req, errors.New("private-cause-sentinel"))
	var problem apperrors.ProblemDetail
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.TraceID != sc.TraceID().String() {
		t.Error("HTTP problem lacks current trace ID")
	}
	if strings.Contains(output.String(), "private-") {
		t.Error("HTTP diagnostics exposed raw request or backend values")
	}
}
