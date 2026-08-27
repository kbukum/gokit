package logging

import (
	"context"
	"log/slog"
)

// contextIDs are the identifiers contextHandler folds from the context into
// structured fields, mirroring the ContextWith* helpers.
var contextIDs = []string{
	FieldTraceID,
	FieldSpanID,
	FieldRequestID,
	FieldUserID,
	FieldCorrelationID,
}

// contextHandler enriches every record with correlation identifiers carried on
// the context (trace, span, request, user, correlation). It is the automatic
// counterpart to (*Logger).WithContext: the *Ctx logging methods propagate the
// context here so identifiers appear without the caller re-deriving a logger.
type contextHandler struct {
	next slog.Handler
}

// newContextHandler wraps next so context identifiers are added at emit time.
func newContextHandler(next slog.Handler) slog.Handler {
	return &contextHandler{next: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	for _, key := range contextIDs {
		if v, ok := ctx.Value(contextKey(key)).(string); ok && v != "" {
			rec.AddAttrs(slog.String(key, v))
		}
	}
	return h.next.Handle(ctx, rec)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}
