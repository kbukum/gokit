package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Handler serves scoped proto events with per-write deadlines and owned teardown. Idle waits between frames carry no
// write deadline.
type Handler struct {
	bus *Bus
	cfg HandlerConfig
}

// NewHandler requires explicit authorization, logger, clock, and positive budgets. Response writers must support deadlines and error-reporting flushes through ResponseController.
func NewHandler(bus *Bus, cfg HandlerConfig) (*Handler, error) {
	if bus == nil || cfg.Authorize == nil || cfg.Logger == nil || util.IsNil(cfg.Clock) || cfg.WriteTimeout <= 0 || cfg.Heartbeat <= 0 {
		return nil, apperrors.InvalidInput("handler", "SSE bus, authorization, logger, clock, and positive timing budgets are required")
	}
	return &Handler{bus: bus, cfg: cfg}, nil
}

// ServeHTTP authenticates before allocating subscription storage and returns immediately on any write, flush, cancellation, or terminal stream outcome.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	access, err := h.cfg.Authorize(r)
	if err != nil {
		h.reject(w, r, err)
		return
	}
	if access.Lifetime != nil && access.Lifetime.Err() != nil {
		h.reject(w, r, apperrors.Unauthorized(""))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub, err := h.bus.Subscribe(ctx, SubscribeRequest{Principal: access.Principal, Route: access.Route, Cursor: r.Header.Get("Last-Event-ID")})
	if err != nil {
		h.reject(w, r, err)
		return
	}
	defer sub.Close()
	rc := http.NewResponseController(w)
	if deadlineErr := rc.SetWriteDeadline(h.cfg.Clock.Now().Add(h.cfg.WriteTimeout)); deadlineErr != nil {
		h.reject(w, r, apperrors.Internal(deadlineErr))
		return
	}
	// Cancellation interrupts a blocked network write as well as the event wait. Wait for the callback before returning the response writer to net/http.
	stop := h.interruptOnCancel(ctx, rc)
	defer stop()
	if access.Lifetime != nil {
		release := context.AfterFunc(access.Lifetime, cancel) //nolint:contextcheck // independent credential lifetime is deliberately joined to the request lifecycle
		defer release()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	connected, err := controlEvent("connected", sub.Connected())
	if err == nil {
		err = h.write(ctx, rc, w, connected.Wire())
	}
	if err == nil {
		err = h.stream(ctx, rc, w, sub)
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		h.cfg.Logger.WarnCtx(r.Context(), "SSE stream ended", map[string]any{"error": err.Error()})
	}
}

func (h *Handler) interruptOnCancel(ctx context.Context, rc *http.ResponseController) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		if err := rc.SetWriteDeadline(h.cfg.Clock.Now()); err != nil {
			h.cfg.Logger.WarnCtx(ctx, "SSE cancellation deadline failed", map[string]any{"error": err.Error()})
		}
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}

func (h *Handler) stream(ctx context.Context, rc *http.ResponseController, w http.ResponseWriter, sub *Subscription) error {
	ticker := time.NewTicker(h.cfg.Heartbeat)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		sub.bus.mu.Lock()
		event, ready, err := sub.nextLocked()
		sub.bus.mu.Unlock()
		if err != nil {
			return err
		}
		if ready {
			if err := h.write(ctx, rc, w, event.Wire()); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sub.wake:
		case <-ticker.C:
			if err := h.write(ctx, rc, w, ": keepalive\n\n"); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) write(ctx context.Context, rc *http.ResponseController, w http.ResponseWriter, frame string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := rc.SetWriteDeadline(h.cfg.Clock.Now().Add(h.cfg.WriteTimeout)); err != nil {
		return err
	}
	// Cancellation may race deadline renewal; recheck after setting the deadline so no new write starts on a revoked stream.
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := io.WriteString(w, frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	if err := rc.Flush(); err != nil {
		return err
	}
	// The budget bounds this write only. HTTP/2 resets a stream whose deadline expires while idle, so disarm it until
	// the next write renews it; cancellation still interrupts any blocked write through interruptOnCancel.
	return rc.SetWriteDeadline(time.Time{})
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Cache-Control", "no-store")
	failure := apperrors.Normalize(err)
	h.cfg.Logger.WarnCtx(r.Context(), "SSE connection rejected", map[string]any{"code": failure.Code})
	body, encodeErr := codec.Encode(codec.CompactJSON(), failure.ToProblemDetail())
	if encodeErr != nil {
		h.cfg.Logger.ErrorCtx(r.Context(), "SSE rejection encoding failed", map[string]any{"error": encodeErr.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	if failure.HTTPStatus() == http.StatusUnauthorized {
		if challenge := authChallengeFor(err); challenge != "" {
			w.Header().Set("WWW-Authenticate", challenge)
		}
	}
	if failure.Retryable && failure.RetryAfter > 0 {
		seconds := failure.RetryAfter / time.Second
		if failure.RetryAfter%time.Second != 0 {
			seconds++
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
	}
	w.WriteHeader(failure.HTTPStatus())
	if _, writeErr := io.WriteString(w, body); writeErr != nil {
		h.cfg.Logger.WarnCtx(r.Context(), "SSE rejection write failed", map[string]any{"error": writeErr.Error()})
	}
}
