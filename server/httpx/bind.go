package httpx

import (
	"github.com/gin-gonic/gin"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/validation"
)

// BindJSON binds a JSON request body into T and validates it with the injected
// StructValidator. Returns a gokit/errors.AppError on parse or validation failure.
func BindJSON[T any](c *gin.Context, sv *validation.StructValidator) (*T, error) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, goerrors.Validation("invalid request body").WithCause(err).
			WithViolations(goerrors.Violation{Reason: goerrors.ViolationInvalidFormat, Message: "invalid request body"})
	}
	if appErr := sv.Validate(req); appErr != nil {
		return nil, appErr
	}
	return &req, nil
}

// BindQuery binds query parameters into T and validates with the injected validator.
func BindQuery[T any](c *gin.Context, sv *validation.StructValidator) (*T, error) {
	var req T
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, goerrors.Validation("invalid query parameters").WithCause(err).
			WithViolations(goerrors.Violation{Reason: goerrors.ViolationInvalidFormat, Message: "invalid query parameters"})
	}
	if appErr := sv.Validate(req); appErr != nil {
		return nil, appErr
	}
	return &req, nil
}
