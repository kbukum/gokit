package logging

import (
	"context"
	"errors"
	"log/slog"
)

// fanoutHandler dispatches every record to a fixed set of downstream handlers,
// so a single logger can write to the default sink, the OTLP bridge, and a
// consumer-supplied handler at once. It is the composition point for
// bring-your-own sinks: additional handlers are appended at construction, never
// through a mutable global.
type fanoutHandler struct {
	handlers []slog.Handler
}

// newFanout builds a fanout over the given handlers. Nil handlers are dropped.
// With a single handler the fanout is transparent.
func newFanout(handlers ...slog.Handler) slog.Handler {
	filtered := make([]slog.Handler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			filtered = append(filtered, h)
		}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &fanoutHandler{handlers: filtered}
}

// Enabled reports whether any downstream handler is enabled for the level.
func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, next := range h.handlers {
		if next.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle forwards the record to every downstream handler, joining their errors
// so one failing sink neither hides the others nor drops their output. It does
// not re-check per-branch Enabled: the emit decision was already made at the top
// of the chain (base level plus any per-module override), so re-filtering here
// would drop records a more-verbose module override deliberately let through and
// make the multi-sink path diverge from the single-sink path.
func (h *fanoutHandler) Handle(ctx context.Context, rec slog.Record) error {
	var errs []error
	for _, next := range h.handlers {
		if err := next.Handle(ctx, rec.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// WithAttrs returns a fanout whose downstream handlers each carry the attrs.
func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		next[i] = hh.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: next}
}

// WithGroup returns a fanout whose downstream handlers each open the group.
func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		next[i] = hh.WithGroup(name)
	}
	return &fanoutHandler{handlers: next}
}
