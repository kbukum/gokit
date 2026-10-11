package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/util"
)

// TracingConfig supplies client tracing for a fixed, application-declared operation.
type TracingConfig struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
	Operation  string
}

// NewTracingTransport decorates a borrowed transport without adding retries or changing timeout policy. The caller must consume or close each response body. Upgrade bodies preserve io.Writer when the underlying body supports it. Operation is a fixed route/procedure label, never a raw request URL.
func NewTracingTransport(next http.RoundTripper, cfg TracingConfig) (http.RoundTripper, error) {
	if util.IsNil(next) || util.IsNil(cfg.Tracer) {
		return nil, apperrors.Validation("HTTP tracing requires a transport and tracer")
	}
	if strings.TrimSpace(cfg.Operation) == "" {
		return nil, apperrors.Validation("HTTP tracing requires a declared operation")
	}
	if util.IsNil(cfg.Propagator) {
		cfg.Propagator = propagation.TraceContext{}
	}
	return &tracingTransport{next: next, cfg: cfg}, nil
}

type tracingTransport struct {
	next http.RoundTripper
	cfg  TracingConfig
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, apperrors.Validation("HTTP request is required")
	}
	ctx := baggage.ContextWithBaggage(req.Context(), baggage.Baggage{})
	ctx, span := t.cfg.Tracer.Start(ctx, t.cfg.Operation, trace.WithSpanKind(trace.SpanKindClient))
	ctx = observability.Correlate(ctx)
	request := req.Clone(ctx)
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	for _, key := range t.cfg.Propagator.Fields() {
		request.Header.Del(key)
	}
	request.Header.Del("Baggage")
	t.cfg.Propagator.Inject(ctx, propagation.HeaderCarrier(request.Header))
	response, err := t.next.RoundTrip(request)
	if err == nil && response == nil {
		err = apperrors.Internal(errors.New("HTTP transport returned no response"))
	}
	if err != nil {
		setHTTPTraceError(span, err)
		span.End()
		return response, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	if response.StatusCode >= http.StatusBadRequest {
		span.SetStatus(codes.Error, http.StatusText(response.StatusCode))
	}
	if response.Body == nil || response.Body == http.NoBody {
		span.End()
	} else {
		body := &traceResponseBody{ReadCloser: response.Body, span: span}
		if writer, ok := response.Body.(io.Writer); ok {
			response.Body = &traceReadWriteBody{traceResponseBody: body, writer: writer}
		} else {
			response.Body = body
		}
	}
	return response, nil
}

// CloseIdleConnections preserves explicit cleanup through http.Client.
func (t *tracingTransport) CloseIdleConnections() {
	if closer, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type traceResponseBody struct {
	io.ReadCloser
	span trace.Span
	once sync.Once
}

type traceReadWriteBody struct {
	*traceResponseBody
	writer io.Writer
}

func (b *traceReadWriteBody) Write(p []byte) (int, error) {
	n, err := b.writer.Write(p)
	if err != nil {
		b.finish(err)
	}
	return n, err
}

func (b *traceResponseBody) finish(err error) {
	b.once.Do(func() {
		if err != nil {
			setHTTPTraceError(b.span, err)
		}
		b.span.End()
	})
}

func (b *traceResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.finish(nil)
	} else if err != nil {
		b.finish(err)
	}
	return n, err
}

func (b *traceResponseBody) Close() error {
	err := b.ReadCloser.Close()
	statusErr := err
	if statusErr == nil {
		statusErr = context.Canceled
	}
	b.finish(statusErr)
	return err
}

func setHTTPTraceError(span trace.Span, err error) {
	code := apperrors.Normalize(err).Code
	span.SetAttributes(attribute.String("error.type", string(code)))
	span.SetStatus(codes.Error, string(code))
}
