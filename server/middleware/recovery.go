package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
)

// Recovery returns middleware that writes a canonical, trace-correlated 500 problem response and safe diagnostic metadata on panic. Install inside tracing so the failure is recorded before span completion; an outer recovery can protect pre-tracing middleware. Panic values and stacks are never logged. A nil logger uses a per-middleware default.
func Recovery(log *logging.Logger) Middleware {
	if log == nil {
		log = logging.NewDefault("middleware")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(logging.ContextWithLogger(r.Context(), log))
			defer func(ctx context.Context) {
				if recovered := recover(); recovered != nil {
					cause, _ := recovered.(error)
					span := observability.SpanFromContext(ctx)
					span.SetAttributes(attribute.String("error.type", string(apperrors.ErrCodeInternal)))
					span.SetStatus(codes.Error, string(apperrors.ErrCodeInternal))
					WriteProblemDetails(w, r, apperrors.Internal(cause))
				}
			}(r.Context())
			next.ServeHTTP(w, r)
		})
	}
}

// GinRecovery returns a Gin middleware that recovers from panics using the injected logger.
// Prefer using Recovery() at the server level via ApplyMiddleware() which covers all routes.
// Use this only when you need recovery on the Gin engine directly.
func GinRecovery(log *logging.Logger) gin.HandlerFunc {
	return GinWrap(Recovery(log))
}
