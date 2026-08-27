package logging

import (
	"context"
	"log/slog"
)

// maskingHandler is an [slog.Handler] middleware that redacts sensitive values
// through a [Masker] before they reach any downstream sink. Masking is applied
// both to attributes bound via WithAttrs and to per-record attributes, so a
// secret cannot leak regardless of where it was attached.
type maskingHandler struct {
	next   slog.Handler
	masker Masker
}

// newMaskingHandler wraps next so every attribute value passes through masker.
// When masker is nil the next handler is returned unwrapped.
func newMaskingHandler(next slog.Handler, masker Masker) slog.Handler {
	if masker == nil {
		return next
	}
	return &maskingHandler{next: next, masker: masker}
}

func (h *maskingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *maskingHandler) Handle(ctx context.Context, rec slog.Record) error {
	masked := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		masked.AddAttrs(h.maskAttr(a))
		return true
	})
	return h.next.Handle(ctx, masked)
}

func (h *maskingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	maskedAttrs := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		maskedAttrs[i] = h.maskAttr(a)
	}
	return &maskingHandler{next: h.next.WithAttrs(maskedAttrs), masker: h.masker}
}

func (h *maskingHandler) WithGroup(name string) slog.Handler {
	return &maskingHandler{next: h.next.WithGroup(name), masker: h.masker}
}

// maskAttr redacts a single attribute. The value is resolved first so a
// [slog.LogValuer] that yields a group is masked recursively; checking the kind
// before resolving would treat such a value as an opaque scalar and could
// forward nested sensitive fields unmasked. Group attributes are masked
// recursively so nested sensitive fields are covered.
func (h *maskingHandler) maskAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		group := v.Group()
		out := make([]slog.Attr, len(group))
		for i, g := range group {
			out[i] = h.maskAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	original := v.String()
	masked := h.masker.MaskValue(a.Key, original)
	if masked == original {
		return slog.Attr{Key: a.Key, Value: v}
	}
	return slog.String(a.Key, masked)
}
