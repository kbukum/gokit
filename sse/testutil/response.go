package testutil

import (
	"net"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

// ResponseWriter extends httptest's recorder with deadline and flush fault injection. Read recorded output after the handler has stopped; deadline accounting is safe during cancellation.
type ResponseWriter struct {
	*httptest.ResponseRecorder
	Deadlines       atomic.Int64
	WriteError      error
	FlushErrorValue error
	DeadlineError   error
	Peer            net.Conn
}

// NewResponseWriter creates a successful deadline-aware response recorder.
func NewResponseWriter() *ResponseWriter {
	return &ResponseWriter{ResponseRecorder: httptest.NewRecorder()}
}

func (w *ResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.Deadlines.Add(1)
	if w.Peer != nil {
		return w.Peer.SetWriteDeadline(deadline)
	}
	return w.DeadlineError
}

func (w *ResponseWriter) Write(p []byte) (int, error) {
	if w.WriteError != nil {
		return 0, w.WriteError
	}
	if w.Peer != nil {
		return w.Peer.Write(p)
	}
	return w.ResponseRecorder.Write(p)
}

func (w *ResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s)) //nolint:gocritic // preserve injected Write failures; calling WriteString here would recurse
}

func (w *ResponseWriter) FlushError() error { return w.FlushErrorValue }
