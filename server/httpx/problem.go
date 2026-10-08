package httpx

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// WriteProblemDetails normalizes and validates err before writing an RFC 9457 response. Nil, invalid and unencodable errors become safe Internal problems. Responses include the request path, no-store and any minimum Retry-After delay. Server failures and write errors are logged only through the request's injected logger; causes never enter the response.
func WriteProblemDetails(w http.ResponseWriter, r *http.Request, err error) {
	failure := apperrors.Normalize(err)
	if failure == nil {
		failure = apperrors.Internal(nil)
	} else if validationErr := failure.Validate(); validationErr != nil {
		failure = apperrors.Internal(errors.Join(err, validationErr))
	}
	problem := failure.ToProblemDetail()
	problem.Instance = r.URL.Path
	body, encodeErr := codec.Encode(codec.CompactJSON(), problem)
	if encodeErr != nil {
		failure = apperrors.Internal(errors.Join(err, encodeErr))
		problem = failure.ToProblemDetail()
		problem.Instance = r.URL.Path
		body, encodeErr = codec.Encode(codec.CompactJSON(), problem)
	}
	log, hasLogger := logging.LoggerFromContext(r.Context())
	if hasLogger && failure.HTTPStatus() >= http.StatusInternalServerError {
		log.ErrorCtx(r.Context(), "HTTP error response", map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": failure.HTTPStatus(),
			"code":   string(failure.Code),
			"error":  failure.Error(),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("Retry-After")
	if encodeErr != nil {
		if hasLogger {
			log.ErrorCtx(r.Context(), "HTTP problem encoding failed", map[string]any{"error": encodeErr.Error()})
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	if failure.Retryable && failure.RetryAfter > 0 {
		seconds := failure.RetryAfter / time.Second
		if failure.RetryAfter%time.Second != 0 {
			seconds++
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
	}
	w.WriteHeader(failure.HTTPStatus())
	if _, writeErr := w.Write([]byte(body)); writeErr != nil && hasLogger { //nolint:gosec // G705: codec HTML-escapes JSON; the response is application/problem+json.
		log.WarnCtx(r.Context(), "HTTP problem response write failed", map[string]any{"error": writeErr.Error()})
	}
}
