package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// samplingHandler rate-limits high-volume logging: it admits an initial burst
// of records each second, then admits only every Nth record thereafter. Its
// clock is injectable so sampling behavior is deterministic under test.
//
// The mutable counter lives in a shared *samplerState so that loggers derived
// via WithAttrs/WithGroup (e.g. per-component loggers) draw from the same rate
// budget as their parent. Were the state copied per derivation, each derived
// logger would get an independent burst allowance and the intended global rate
// limit would not hold.
type samplingHandler struct {
	next       slog.Handler
	burst      uint64
	thereafter uint64
	now        func() time.Time
	state      *samplerState
}

// samplerState holds the mutable, concurrency-safe sampling counter shared
// across a handler and every handler derived from it.
type samplerState struct {
	mu          sync.Mutex
	periodStart time.Time
	count       uint64
}

// newSamplingHandler wraps next with burst sampling. When sampling is disabled
// (or the rates are zero) the next handler is returned unwrapped. now defaults
// to time.Now when nil.
func newSamplingHandler(next slog.Handler, cfg SamplingConfig, now func() time.Time) slog.Handler {
	if !cfg.Enabled || cfg.InitialRate <= 0 {
		return next
	}
	if now == nil {
		now = time.Now
	}
	return &samplingHandler{
		next:       next,
		burst:      uint64(cfg.InitialRate),
		thereafter: uint64(max(cfg.ThereafterRate, 0)),
		now:        now,
		state:      &samplerState{},
	}
}

func (h *samplingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *samplingHandler) Handle(ctx context.Context, rec slog.Record) error {
	if !h.admit() {
		return nil
	}
	return h.next.Handle(ctx, rec)
}

// admit reports whether the current record passes the sampler.
func (h *samplingHandler) admit() bool {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	now := h.now()
	if h.state.periodStart.IsZero() || now.Sub(h.state.periodStart) >= time.Second {
		h.state.periodStart = now
		h.state.count = 0
	}
	h.state.count++

	if h.state.count <= h.burst {
		return true
	}
	if h.thereafter == 0 {
		return false
	}
	return (h.state.count-h.burst)%h.thereafter == 0
}

func (h *samplingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &samplingHandler{next: h.next.WithAttrs(attrs), burst: h.burst, thereafter: h.thereafter, now: h.now, state: h.state}
}

func (h *samplingHandler) WithGroup(name string) slog.Handler {
	return &samplingHandler{next: h.next.WithGroup(name), burst: h.burst, thereafter: h.thereafter, now: h.now, state: h.state}
}
