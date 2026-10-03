package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// MaxBufferedResponseBytes bounds REST timeout buffering. Streaming responses must bypass this middleware.
const MaxBufferedResponseBytes = 10 * 1024 * 1024

// Timeout returns middleware that bounds each request to d. When the deadline
// elapses the request context is canceled — so downstream calls that honor the
// context stop — and the client receives a 503 with a Problem Details
// (application/problem+json) body, so JSON clients can parse the response even
// under a nosniff policy.
//
// A non-positive d disables the middleware (returns the handler unchanged).
// The timeout applies to every wrapped route, so do not enable it in front of
// long-lived streaming handlers (e.g. SSE); mount those without it.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		if d <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			buf := &bufferedResponseWriter{header: make(http.Header), status: http.StatusOK, ctx: ctx, cancel: cancel, overflow: make(chan struct{})}
			done := make(chan struct{})
			go func() {
				defer close(done)
				next.ServeHTTP(buf, r.WithContext(ctx))
			}()

			select {
			case <-done:
			case <-ctx.Done():
			case <-buf.overflow:
			}
			if err := buf.finish(ctx.Err()); err != nil {
				failure := apperrors.New(apperrors.ErrCodeServiceUnavailable, "request timeout")
				if !errors.Is(err, http.ErrHandlerTimeout) {
					failure = apperrors.New(apperrors.ErrCodeServiceUnavailable, "response exceeds buffering limit")
				}
				if err := writeTimeoutFailure(w, r, failure); err != nil {
					logTimeoutWrite(r.Context(), err)
				}
			} else if err := buf.flushTo(w); err != nil {
				logTimeoutWrite(r.Context(), err)
			}
		})
	}
}

func writeTimeoutFailure(w http.ResponseWriter, r *http.Request, failure *apperrors.AppError) error {
	pd := failure.ToProblemDetail()
	pd.Instance = r.URL.Path
	body, err := json.Marshal(pd)
	if err != nil {
		http.Error(w, "failed to encode timeout response", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, err = w.Write(body)
	return err
}

func logTimeoutWrite(ctx context.Context, err error) {
	if logger, ok := logging.LoggerFromContext(ctx); ok {
		logger.ErrorCtx(ctx, "Timeout response write failed", map[string]any{"error": err.Error()})
	}
}

// bufferedResponseWriter records a handler's response so the timeout middleware
// can either replay it once the handler finishes or discard it if the deadline
// fires first. Discarding avoids a partially written response racing with the
// timeout body on the real writer.
type bufferedResponseWriter struct {
	mu          sync.Mutex
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
	ctx         context.Context
	cancel      context.CancelFunc
	overflow    chan struct{}
	closed      bool
	err         error
}

func (b *bufferedResponseWriter) Header() http.Header { return b.header }

func (b *bufferedResponseWriter) WriteHeader(status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wroteHeader || b.closed {
		return
	}
	b.status = status
	b.wroteHeader = true
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return 0, b.err
	}
	if b.closed || b.ctx.Err() != nil {
		return 0, http.ErrHandlerTimeout
	}
	if len(p) > MaxBufferedResponseBytes-b.body.Len() {
		b.err = apperrors.New(apperrors.ErrCodeServiceUnavailable, "response exceeds buffering limit")
		b.closed = true
		b.body = bytes.Buffer{}
		close(b.overflow)
		b.cancel()
		return 0, b.err
	}
	b.wroteHeader = true
	return b.body.Write(p)
}

func (b *bufferedResponseWriter) finish(contextErr error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.err == nil && contextErr != nil {
		b.err = http.ErrHandlerTimeout
	}
	if b.err != nil {
		b.body = bytes.Buffer{}
	}
	return b.err
}

// flushTo replays the recorded response onto the real writer.
func (b *bufferedResponseWriter) flushTo(w http.ResponseWriter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	dst := w.Header()
	for k, vv := range b.header {
		dst[k] = vv
	}
	w.WriteHeader(b.status)
	_, err := w.Write(b.body.Bytes())
	return err
}
