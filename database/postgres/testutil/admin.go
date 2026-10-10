package testutil

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
)

// AdminConfig returns an owned snapshot for isolated test administration, never production use. It preserves the fixture's verified connection settings and must not be logged.
func (f *Fixture) AdminConfig() (*pgx.ConnConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || f.terminate == nil || f.admin == nil {
		return nil, apperrors.New(apperrors.ErrCodeServiceUnavailable, "PostgreSQL fixture is closed or not initialized")
	}
	return f.admin.Copy(), nil
}

func snapshotAdmin(ctx context.Context, db *database.DB) (config *pgx.ConnConfig, err error) {
	pool, err := db.GormDB.DB()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	err = conn.Raw(func(raw any) error {
		driver, ok := raw.(*stdlib.Conn)
		if !ok {
			return apperrors.New(apperrors.ErrCodeInternal, "Fixture connection is not a PostgreSQL driver")
		}
		config = driver.Conn().Config()
		return nil
	})
	return config, err
}
