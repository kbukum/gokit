package httpx

import (
	"github.com/gin-gonic/gin"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/validation"
)

// BindJSON binds a JSON request body into T and validates it via gokit/validation struct tags. Returns a gokit/errors.AppError on parse or validation failure.
func BindJSON[T any](c *gin.Context) (*T, error) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, goerrors.Validation("invalid request body").WithDetail("error", err.Error())
	}
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	return &req, nil
}

// BindQuery binds query parameters into T and validates.
func BindQuery[T any](c *gin.Context) (*T, error) {
	var req T
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, goerrors.Validation("invalid query parameters").WithDetail("error", err.Error())
	}
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	return &req, nil
}
