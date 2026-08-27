package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// samplingHandler rate-limits high-volume logging: per severity level it admits
// an initial burst of records each second, then admits only every Nth record
// thereafter. Its clock is injectable so sampling behavior is deterministic
// under test.
//
// The mutable per-level counters live in a shared *samplerState so that loggers
// derived via WithAttrs/WithGroup (e.g. per-component loggers) draw from the
// same rate budget as their parent. Were the state copied per derivation, each
// derived logger would get an independent burst allowance and the intended
// global rate limit would not hold.
type samplingHandler struct {
	next       slog.Handler
	burst      uint64
	thereafter uint64
	now        func() time.Time
	state      *samplerState
}

// samplerState holds the mutable, concurrency-safe sampling counters shared
// across a handler and every handler derived from it. Each severity level keeps
// its own window and counter so a burst of low-severity records (info/debug)
// cannot consume the budget and suppress a later error record.
type samplerState struct {
	mu       sync.Mutex
	perLevel map[slog.Level]*levelWindow
}

// levelWindow is the per-second counter for a single severity level.
type levelWindow struct {
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
		state:      &samplerState{perLevel: make(map[slog.Level]*levelWindow)},
	}
}

func (h *samplingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *samplingHandler) Handle(ctx context.Context, rec slog.Record) error {
	if !h.admit(rec.Level) {
		return nil
	}
	return h.next.Handle(ctx, rec)
}

// admit reports whether the current record passes the sampler for its level.
// Each severity level is counted independently so its budget is isolated.
func (h *samplingHandler) admit(level slog.Level) bool {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	w := h.state.perLevel[level]
	if w == nil {
		w = &levelWindow{}
		h.state.perLevel[level] = w
	}

	now := h.now()
	if w.periodStart.IsZero() || now.Sub(w.periodStart) >= time.Second {
		w.periodStart = now
		w.count = 0
	}
	w.count++

	if w.count <= h.burst {
		return true
	}
	if h.thereafter == 0 {
		return false
	}
	return (w.count-h.burst)%h.thereafter == 0
}

func (h *samplingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &samplingHandler{next: h.next.WithAttrs(attrs), burst: h.burst, thereafter: h.thereafter, now: h.now, state: h.state}
}

func (h *samplingHandler) WithGroup(name string) slog.Handler {
	return &samplingHandler{next: h.next.WithGroup(name), burst: h.burst, thereafter: h.thereafter, now: h.now, state: h.state}
}
