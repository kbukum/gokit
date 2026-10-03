package database

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// PoolConfigurer lets a dialector tighten the application's connection budget.
type PoolConfigurer interface {
	ConfigurePool(*sql.DB)
}

// ReadPoolDialect optionally supplies a separate read-only pool. Nil keeps reads on the primary pool.
type ReadPoolDialect interface {
	ReadDialector() gorm.Dialector
}

// ReadOnly returns the dedicated reader where supported, otherwise the primary pool. It does not make arbitrary backends read-only; use WithReadOnlyTransaction for a transaction-level guarantee.
func (d *DB) ReadOnly(ctx context.Context) *gorm.DB {
	if d.reader != nil {
		return d.reader.WithContext(ctx)
	}
	return d.GormDB.WithContext(ctx)
}
