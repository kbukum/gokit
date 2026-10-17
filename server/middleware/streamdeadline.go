package middleware

import (
	"net/http"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// StreamWriteDeadline returns middleware for long-lived streaming routes (Connect or gRPC server streams, chunked
// responses) served by a server with a WriteTimeout.
//
// Under HTTP/2, http.Server.WriteTimeout is an absolute deadline per stream that successful writes do not renew, so a
// healthy stream is reset once it elapses. The middleware disarms that deadline when the request starts and instead
// bounds each Write and Flush by budget, disarming it again afterwards so an idle stream is never reset. Stream
// lifetime then belongs to the handler's context. Mount it only on streaming routes; unary routes keep the server
// deadline.
//
// A writer that cannot set deadlines is refused with 500 before the handler runs.
func StreamWriteDeadline(budget time.Duration, clock util.Clock) (Middleware, error) {
	if budget <= 0 || util.IsNil(clock) {
		return nil, apperrors.InvalidInput("stream write deadline", "A positive write budget and a clock are required")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Time{}); err != nil {
				WriteProblemDetails(w, r, apperrors.Internal(err))
				return
			}
			next.ServeHTTP(&deadlineWriter{ResponseWriter: w, rc: rc, budget: budget, clock: clock}, r)
		})
	}, nil
}

type deadlineWriter struct {
	http.ResponseWriter
	rc     *http.ResponseController
	budget time.Duration
	clock  util.Clock
}

func (d *deadlineWriter) Write(p []byte) (int, error) {
	if err := d.rc.SetWriteDeadline(d.clock.Now().Add(d.budget)); err != nil {
		return 0, err
	}
	n, err := d.ResponseWriter.Write(p)
	if disarm := d.rc.SetWriteDeadline(time.Time{}); err == nil {
		err = disarm
	}
	return n, err
}

func (d *deadlineWriter) Flush() {
	_ = d.FlushError()
}

// FlushError lets http.ResponseController report flush and deadline failures.
func (d *deadlineWriter) FlushError() error {
	if err := d.rc.SetWriteDeadline(d.clock.Now().Add(d.budget)); err != nil {
		return err
	}
	err := d.rc.Flush()
	if disarm := d.rc.SetWriteDeadline(time.Time{}); err == nil {
		err = disarm
	}
	return err
}

// Unwrap lets http.ResponseController reach the underlying writer's optional interfaces.
func (d *deadlineWriter) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}
