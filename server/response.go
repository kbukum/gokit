package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// DataResponse is the standard success envelope.
type DataResponse struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

// Meta carries pagination or other response metadata.
type Meta struct {
	Page       int `json:"page,omitempty" example:"1"`
	PageSize   int `json:"pageSize,omitempty" example:"20"`
	Total      int `json:"total,omitempty" example:"100"`
	TotalPages int `json:"totalPages,omitempty" example:"5"`
}

// RespondWithError inspects err: if it is an *apperrors.AppError the status and
// structured body are derived automatically; otherwise a generic 500 is sent.
// The response uses Content-Type: application/problem+json per RFC 9457.
//
// 5xx responses are logged at Error level (with method, path, status, and
// the underlying error chain) so operators have a server-side trail even
// when the client only sees a generic problem detail. Closes F-082 sub-finding.
// 4xx responses are not logged here — that is the caller's call (for noisy
// validation errors, the caller can choose to log at Debug).
func RespondWithError(c *gin.Context, err error) {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		appErr = apperrors.Internal(err)
	}
	pd := appErr.ToProblemDetail()
	pd.Instance = c.Request.URL.Path

	if appErr.HTTPStatus >= http.StatusInternalServerError {
		logging.Error("HTTP error response", map[string]interface{}{
			"method": c.Request.Method,
			"path":   c.Request.URL.Path,
			"status": appErr.HTTPStatus,
			"code":   string(appErr.Code),
			"error":  err.Error(),
		})
	}

	c.Header("Content-Type", "application/problem+json")
	c.JSON(appErr.HTTPStatus, pd)
}

// RespondOK sends a 200 response wrapping data.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, DataResponse{Data: data})
}

// RespondOKWithMeta sends a 200 response with data and metadata.
func RespondOKWithMeta(c *gin.Context, data any, meta *Meta) {
	c.JSON(http.StatusOK, DataResponse{Data: data, Meta: meta})
}

// RespondCreated sends a 201 response wrapping data.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, DataResponse{Data: data})
}

// RespondNoContent sends a 204 with no body.
func RespondNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// RespondAccepted sends a 202 response wrapping data.
func RespondAccepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, DataResponse{Data: data})
}

// --- Convenience error response helpers ---

// RespondBadRequest sends a 400 response with the given message.
func RespondBadRequest(c *gin.Context, message string) {
	RespondWithError(c, apperrors.InvalidInput("", message))
}

// RespondNotFound sends a 404 response for the given resource.
func RespondNotFound(c *gin.Context, resource string) {
	RespondWithError(c, apperrors.NotFound(resource, ""))
}

// RespondUnauthorized sends a 401 response.
func RespondUnauthorized(c *gin.Context, message string) {
	RespondWithError(c, apperrors.Unauthorized(message))
}

// RespondForbidden sends a 403 response.
func RespondForbidden(c *gin.Context, message string) {
	RespondWithError(c, apperrors.Forbidden(message))
}

// RespondInternalError sends a 500 response for the given cause.
func RespondInternalError(c *gin.Context, cause error) {
	RespondWithError(c, apperrors.Internal(cause))
}
