package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
)

// DriverFunc creates an owned migration session. Every driver operation must use the supplied context; Close must release only the session, never the shared pool.
type DriverFunc func(context.Context, *sql.DB) (migratedb.Driver, error)

// Config defines a synchronous migration run. SQL files use VERSION_name.up.sql and VERSION_name.down.sql. Timeout defaults to two minutes; each file is limited to 1 MiB.
type Config struct {
	DB      *gorm.DB
	FS      fs.FS
	Path    string
	Driver  DriverFunc
	Timeout time.Duration
}

const maxMigrationBytes = 1 << 20

// Up applies pending migrations in source order.
func (c Config) Up(ctx context.Context) error {
	return c.run(ctx, "migrate up", func(s source.Driver, d migratedb.Driver) error {
		return advance(s, d, 1, -1)
	})
}

// Down rolls back every applied migration.
func (c Config) Down(ctx context.Context) error {
	return c.run(ctx, "migrate down", func(s source.Driver, d migratedb.Driver) error {
		return advance(s, d, -1, -1)
	})
}

// Steps applies n migrations, forwards for positive n and backwards for negative n.
func (c Config) Steps(ctx context.Context, n int) error {
	return c.run(ctx, "migrate steps", func(s source.Driver, d migratedb.Driver) error {
		direction := 1
		if n < 0 {
			direction = -1
			if n == -n {
				return apperrors.InvalidInput("steps", "migration step count is too large")
			}
			n = -n
		}
		return advance(s, d, direction, n)
	})
}

// Version reports the stored schema version and dirty flag. An empty schema returns migrate.ErrNilVersion.
func (c Config) Version(ctx context.Context) (version uint, dirty bool, err error) {
	err = c.run(ctx, "migration version", func(_ source.Driver, d migratedb.Driver) error {
		current, isDirty, versionErr := d.Version()
		if versionErr != nil {
			return versionErr
		}
		dirty = isDirty
		if current < 0 {
			return migrate.ErrNilVersion
		}
		version = uint(current)
		return nil
	})
	return version, dirty, err
}

// Ready rejects missing, dirty or unexpected schema versions. Composition must call it in addition to the connection health check.
func (c Config) Ready(ctx context.Context, expected uint) error {
	version, dirty, err := c.Version(ctx)
	if err != nil {
		return err
	}
	if dirty || version != expected {
		return apperrors.New(apperrors.ErrCodeDatabaseError, "database schema is not ready").
			WithCause(fmt.Errorf("schema version=%d dirty=%t expected=%d", version, dirty, expected))
	}
	return nil
}

// Reset destroys all application tables and reapplies migrations. Use only in development and tests.
func (c Config) Reset(ctx context.Context) error {
	return c.run(ctx, "migrate reset", func(s source.Driver, d migratedb.Driver) error {
		if err := d.Drop(); err != nil {
			return fmt.Errorf("migrate drop: %w", err)
		}
		if err := advance(s, d, 1, -1); err != nil {
			return fmt.Errorf("migrate up after reset: %w", err)
		}
		return nil
	})
}

func (c Config) run(ctx context.Context, operation string, fn func(source.Driver, migratedb.Driver) error) error {
	if c.DB == nil || c.Driver == nil || c.FS == nil || c.Path == "" || c.Timeout < 0 {
		return apperrors.InvalidInput("migration", "database, source, path and driver are required; timeout cannot be negative")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	_, err := resilience.Execute(ctx, resilience.NewPolicy().WithTimeout(timeout), func(ctx context.Context) (struct{}, error) {
		return struct{}{}, c.execute(ctx, fn)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (c Config) execute(ctx context.Context, fn func(source.Driver, migratedb.Driver) error) (err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	s, err := iofs.New(c.FS, c.Path)
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	defer func() { err = errors.Join(err, s.Close()) }()
	pool, err := c.DB.DB()
	if err != nil {
		return err
	}
	d, err := c.Driver(ctx, pool)
	if err != nil {
		return fmt.Errorf("create database driver: %w", err)
	}
	defer func() { err = errors.Join(err, d.Close()) }()
	if lockErr := d.Lock(); lockErr != nil {
		return lockErr
	}
	defer func() { err = errors.Join(err, d.Unlock()) }()
	return fn(s, d)
}

func advance(s source.Driver, d migratedb.Driver, direction, count int) error {
	current, dirty, err := d.Version()
	if err != nil {
		return err
	}
	if dirty {
		return migrate.ErrDirty{Version: current}
	}
	for applied := 0; count < 0 || applied < count; applied++ {
		version, target, err := nextVersion(s, current, direction)
		if errors.Is(err, fs.ErrNotExist) {
			if count > 0 && applied > 0 {
				return migrate.ErrShortLimit{Short: uint(count - applied)}
			}
			return nil
		}
		if err != nil {
			return err
		}
		text, err := readMigration(s, version, direction)
		if err != nil {
			return err
		}
		if err := d.SetVersion(target, true); err != nil {
			return err
		}
		if err := d.Run(strings.NewReader(text)); err != nil {
			return err
		}
		if err := d.SetVersion(target, false); err != nil {
			return err
		}
		current = target
	}
	return nil
}

func nextVersion(s source.Driver, current, direction int) (sourceVersion uint, targetVersion int, resultErr error) {
	if direction > 0 {
		var version uint
		var err error
		if current < 0 {
			version, err = s.First()
		} else {
			version, err = s.Next(uint(current))
		}
		if int(version) < 0 {
			return 0, 0, apperrors.InvalidInput("migration", "schema version exceeds the supported range")
		}
		return version, int(version), err
	}
	if current < 0 {
		return 0, 0, fs.ErrNotExist
	}
	previous, err := s.Prev(uint(current))
	if errors.Is(err, fs.ErrNotExist) {
		return uint(current), -1, nil
	}
	return uint(current), int(previous), err
}

func readMigration(s source.Driver, version uint, direction int) (text string, err error) {
	var reader io.ReadCloser
	if direction > 0 {
		reader, _, err = s.ReadUp(version)
	} else {
		reader, _, err = s.ReadDown(version)
	}
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	bytes, err := io.ReadAll(io.LimitReader(reader, maxMigrationBytes+1))
	if err != nil {
		return "", err
	}
	if len(bytes) > maxMigrationBytes {
		return "", apperrors.InvalidInput("migration", "migration file exceeds 1 MiB")
	}
	return string(bytes), nil
}
