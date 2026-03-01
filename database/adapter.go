package database

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// compile-time assertion
var _ provider.Provider = (*DB)(nil)

// Name returns the adapter name (implements provider.Provider).
func (db *DB) Name() string {
	return db.cfg.Name
}

// IsAvailable checks if the database connection is healthy (implements provider.Provider).
func (db *DB) IsAvailable(ctx context.Context) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return false
	}
	return db.PingContext(ctx) == nil
}
