package cache

import "fmt"

// ConfigTypeError reports an adapter-specific config type mismatch.
type ConfigTypeError struct {
	Provider string
	Expected string
	Actual   any
}

func (e *ConfigTypeError) Error() string {
	return fmt.Sprintf("%s: expected %s, got %T", e.Provider, e.Expected, e.Actual)
}
