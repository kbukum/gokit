package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

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

			buf := &bufferedResponseWriter{header: make(http.Header), status: http.StatusOK}
			done := make(chan struct{})
			go func() {
				defer close(done)
				next.ServeHTTP(buf, r.WithContext(ctx))
			}()

			select {
			case <-done:
				buf.flushTo(w)
			case <-ctx.Done():
				writeTimeout(w, r)
			}
		})
	}
}

// writeTimeout emits the 503 Problem Details response for an elapsed deadline.
func writeTimeout(w http.ResponseWriter, r *http.Request) {
	pd := apperrors.New(apperrors.ErrCodeServiceUnavailable, "request timeout", http.StatusServiceUnavailable).ToProblemDetail()
	pd.Instance = r.URL.Path
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)
	body, _ := json.Marshal(pd)
	_, _ = w.Write(body)
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
}

func (b *bufferedResponseWriter) Header() http.Header { return b.header }

func (b *bufferedResponseWriter) WriteHeader(status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wroteHeader {
		return
	}
	b.status = status
	b.wroteHeader = true
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wroteHeader = true
	return b.body.Write(p)
}

// flushTo replays the recorded response onto the real writer.
func (b *bufferedResponseWriter) flushTo(w http.ResponseWriter) {
	b.mu.Lock()
	defer b.mu.Unlock()
	dst := w.Header()
	for k, vv := range b.header {
		dst[k] = vv
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}
