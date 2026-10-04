package testhost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
)

type heldStatus struct {
	release chan struct{}
	once    sync.Once
	claimed atomic.Bool
	ready   atomic.Bool
}

func (s *heldStatus) close() { s.once.Do(func() { close(s.release) }) }

type statusCapture struct {
	headers http.Header
	status  int
	body    bytes.Buffer
	err     error
}

func (c *statusCapture) Header() http.Header { return c.headers }
func (c *statusCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *statusCapture) Write(data []byte) (int, error) {
	if c.body.Len()+len(data) > 4096 {
		c.err = apperrors.New(apperrors.ErrCodeInternal, "Fixture status response exceeded its budget")
		return 0, c.err
	}
	c.WriteHeader(http.StatusOK)
	return c.body.Write(data)
}

// fenceStatus holds one already-authoritative response to reproduce logout/status ordering. It cannot hold more than 4 KiB or outlive its five-second request budget.
func (h *Host) fenceStatus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hold := h.status.Load()
		if r.URL.Path != "/auth/session" || hold == nil || !hold.claimed.CompareAndSwap(false, true) {
			next.ServeHTTP(w, r)
			return
		}
		defer h.status.CompareAndSwap(hold, nil)
		capture := &statusCapture{headers: make(http.Header)}
		next.ServeHTTP(capture, r)
		hold.ready.Store(true)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		select {
		case <-ctx.Done():
			return
		case <-hold.release:
		}
		if capture.err != nil {
			h.writeError(w, r, capture.err)
			return
		}
		for name, values := range capture.headers {
			w.Header()[name] = values
		}
		if capture.status == 0 {
			capture.status = http.StatusOK
		}
		w.WriteHeader(capture.status)
		if _, err := io.Copy(w, &capture.body); err != nil {
			h.log.ErrorCtx(r.Context(), "Held fixture status write failed", map[string]any{"error": err.Error()})
		}
	})
}

func (h *Host) state(w http.ResponseWriter, r *http.Request) {
	hold := h.status.Load()
	data, err := codec.Encode(codec.CompactJSON(), struct {
		StatusPending bool `json:"statusPending"`
	}{hold != nil && hold.ready.Load()})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.WriteString(w, data); err != nil {
		h.log.ErrorCtx(r.Context(), "Fixture state write failed", map[string]any{"error": err.Error()})
	}
}
