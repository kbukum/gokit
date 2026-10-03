package sqlite

import (
	"database/sql"
	"net/url"
	"strings"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

type dialector struct {
	gormsqlite.Dialector
	dsn      string
	readOnly bool
	inMemory bool
}

func (*dialector) Name() string { return Name }

func (d *dialector) Initialize(db *gorm.DB) error {
	if d.dsn == "" {
		return apperrors.InvalidInput("dsn", "SQLite requires a filename or :memory:")
	}
	path, raw, _ := strings.Cut(d.dsn, "?")
	values, err := url.ParseQuery(raw)
	if err != nil {
		return apperrors.InvalidInput("dsn", "invalid SQLite connection options").WithCause(err)
	}
	d.inMemory = path == ":memory:" || path == "file::memory:" || values.Get("mode") == "memory"
	values.Set("_busy_timeout", "5000")
	values.Set("_journal_mode", "WAL")
	values.Set("_foreign_keys", "on")
	values.Set("_txlock", "immediate")
	values.Set("_query_only", "false")
	// Remove driver aliases so they cannot override the enforced settings.
	for _, alias := range []string{"_timeout", "_journal", "_fk"} {
		values.Del(alias)
	}
	if d.readOnly {
		values.Set("_query_only", "true")
		values.Set("_txlock", "deferred")
	}
	d.Dialector = gormsqlite.Dialector{DSN: path + "?" + values.Encode()}
	if initErr := d.Dialector.Initialize(db); initErr != nil {
		return initErr
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	d.ConfigurePool(pool)
	return nil
}

func (d *dialector) ConfigurePool(pool *sql.DB) {
	limit := 1
	if d.readOnly {
		limit = 4
	}
	pool.SetMaxOpenConns(limit)
	pool.SetMaxIdleConns(limit)
	if d.inMemory {
		// Closing the only connection would destroy the database.
		pool.SetConnMaxLifetime(0)
		pool.SetConnMaxIdleTime(0)
	}
}

func (d *dialector) ReadDialector() gorm.Dialector {
	if d.readOnly || d.inMemory {
		return nil
	}
	return &dialector{dsn: d.dsn, readOnly: true}
}
