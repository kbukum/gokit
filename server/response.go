package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/skillsenselab/gokit/errors"
)

// DataResponse is the standard success envelope.
type DataResponse struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

// Meta carries pagination or other response metadata.
type Meta struct {
	Page       int `json:"page,omitempty"`
	PageSize   int `json:"pageSize,omitempty"`
	Total      int `json:"total,omitempty"`
	TotalPages int `json:"totalPages,omitempty"`
}

// RespondWithError inspects err: if it is an *apperrors.AppError the status and
// structured body are derived automatically; otherwise a generic 500 is sent.
func RespondWithError(c *gin.Context, err error) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, appErr.ToResponse())
		return
	}
	c.JSON(http.StatusInternalServerError, apperrors.Internal(err).ToResponse())
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
