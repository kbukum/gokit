package httpx

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	goerrors "github.com/kbukum/gokit/errors"
)

// ParsePathUUID parses a UUID from a gin path parameter.
// Returns a validation AppError on failure.
func ParsePathUUID(c *gin.Context, name string) (uuid.UUID, error) {
	raw := c.Param(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, goerrors.Validation("invalid "+name).WithDetail(name, raw)
	}
	return id, nil
}

// ParsePathInt parses an integer from a gin path parameter.
func ParsePathInt(c *gin.Context, name string) (int, error) {
	raw := c.Param(name)
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, goerrors.Validation("invalid "+name).WithDetail(name, raw)
	}
	return v, nil
}

// IntQuery parses an integer query parameter with a default fallback.
func IntQuery(c *gin.Context, key string, def int) int {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// StringQuery returns a query parameter or the default value.
func StringQuery(c *gin.Context, key string, def string) string {
	s := c.Query(key)
	if s == "" {
		return def
	}
	return s
}

// BoolQuery parses a boolean query parameter with a default fallback.
func BoolQuery(c *gin.Context, key string, def bool) bool {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return v
}
