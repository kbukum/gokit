package storage

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// compile-time assertion — Component implements provider.Provider
var _ provider.Provider = (*Component)(nil)

// IsAvailable checks if the storage backend is initialized (implements provider.Provider).
func (c *Component) IsAvailable(_ context.Context) bool {
	_, err := c.Source()
	return err == nil
}
