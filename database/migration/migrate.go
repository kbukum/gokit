package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
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

// DriverFunc creates an owned migration session that records its version in versionTable. Every driver operation must use the supplied context; Close must release only the session, never the shared pool.
type DriverFunc func(ctx context.Context, pool *sql.DB, table Table) (migratedb.Driver, error)

// Config defines a synchronous migration run. SQL files use VERSION_name.up.sql and VERSION_name.down.sql. Timeout defaults to two minutes; each file is limited to 1 MiB.
//
// Table identifies each set's metadata explicitly. An empty Name defaults to DefaultVersionTable.
type Config struct {
	DB      *gorm.DB
	FS      fs.FS
	Path    string
	Driver  DriverFunc
	Timeout time.Duration
	Table   Table
}

// DefaultVersionTable stores the application's migration version.
const DefaultVersionTable = "schema_migrations"

const (
	maxMigrationBytes  = 1 << 20
	versionTableSuffix = "_" + DefaultVersionTable
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// IsIdentifier reports whether name is a portable unquoted SQL identifier: a lowercase letter followed by at most 62
// lowercase letters, digits or underscores. It is the one grammar for schemas and metadata tables; backends add their
// reserved names on top.
func IsIdentifier(name string) bool { return identifierPattern.MatchString(name) }

// IsVersionTable reports whether name follows the migration version-table naming convention.
func IsVersionTable(name string) bool {
	return IsIdentifier(name) && (name == DefaultVersionTable || strings.HasSuffix(name, versionTableSuffix))
}

func (c Config) versionTable() (Table, error) {
	table := c.Table
	if table.Name == "" {
		table.Name = DefaultVersionTable
	}
	return table, table.Validate()
}

// Up applies every pending migration in source order. A dirty schema or a stored version the source does not contain
// (a future or removed migration) is rejected before any SQL runs.
func (c Config) Up(ctx context.Context) error {
	return c.run(ctx, "migrate up", func(s source.Driver, d migratedb.Driver) error {
		return migrateUp(s, d, nil)
	})
}

// Apply migrates forward to exactly target. It rejects a dirty schema, a stored version that is unknown to the source or
// beyond target, a target the source does not contain, and unreadable or oversized pending files, all before any SQL
// runs. It never rolls back.
func (c Config) Apply(ctx context.Context, target uint) error {
	return c.run(ctx, "migrate apply", func(s source.Driver, d migratedb.Driver) error {
		return migrateUp(s, d, &target)
	})
}

// Down rolls back every applied migration.
func (c Config) Down(ctx context.Context) error {
	return c.run(ctx, "migrate down", func(s source.Driver, d migratedb.Driver) error {
		current, err := preflight(s, d, nil)
		if err != nil {
			return err
		}
		return advance(s, d, current, -1, -1, -1)
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
		current, err := preflight(s, d, nil)
		if err != nil {
			return err
		}
		return advance(s, d, current, direction, n, -1)
	})
}

// Version reports the stored schema version and dirty flag. An empty schema returns migrate.ErrNilVersion.
func (c Config) Version(ctx context.Context) (version uint, dirty bool, err error) {
	err = c.inspect(ctx, "migration version", func(d migratedb.Driver) error {
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

// Ready rejects missing, dirty or unexpected schema versions as DATABASE_ERROR "database schema is not ready"; read
// failures keep their own classification. Composition must call it in addition to the connection health check.
func (c Config) Ready(ctx context.Context, expected uint) error {
	version, dirty, err := c.Version(ctx)
	if errors.Is(err, migrate.ErrNilVersion) {
		return notReady(err)
	}
	if err != nil {
		return err
	}
	if dirty || version != expected {
		return notReady(fmt.Errorf("schema version=%d dirty=%t expected=%d", version, dirty, expected))
	}
	return nil
}

// Reset destroys every table in the version table's schema (SQLite: the main database), including other migration sets
// and their version tables that share it, then reapplies this set. Use only in development and tests.
func (c Config) Reset(ctx context.Context) error {
	return c.run(ctx, "migrate reset", func(s source.Driver, d migratedb.Driver) error {
		if err := d.Drop(); err != nil {
			return fmt.Errorf("migrate drop: %w", err)
		}
		if err := advance(s, d, migratedb.NilVersion, 1, -1, -1); err != nil {
			return fmt.Errorf("migrate up after reset: %w", err)
		}
		return nil
	})
}

// inspect reads the version without the migration lock or any DDL, so a least-privilege runtime role can check readiness and never waits for a running migration. Version writes are transactional and a running migration records itself dirty first, so an unlocked read never reports a half-applied schema as ready.
func (c Config) inspect(ctx context.Context, operation string, fn func(migratedb.Driver) error) error {
	versionTable, timeout, err := c.settings(false)
	if err != nil {
		return err
	}
	_, err = resilience.Execute(ctx, resilience.NewPolicy().WithTimeout(timeout), func(ctx context.Context) (struct{}, error) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return struct{}{}, ctxErr
		}
		pool, poolErr := c.DB.DB()
		if poolErr != nil {
			return struct{}{}, poolErr
		}
		d, driverErr := c.Driver(ctx, pool, versionTable)
		if driverErr != nil {
			return struct{}{}, fmt.Errorf("create database driver: %w", driverErr)
		}
		return struct{}{}, errors.Join(fn(d), d.Close())
	})
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (c Config) settings(sourceRequired bool) (versionTable Table, timeout time.Duration, err error) {
	if c.DB == nil || c.Driver == nil || c.Timeout < 0 {
		return Table{}, 0, apperrors.InvalidInput("migration", "database and driver are required; timeout cannot be negative")
	}
	if sourceRequired && (c.FS == nil || c.Path == "") {
		return Table{}, 0, apperrors.InvalidInput("migration", "source and path are required")
	}
	versionTable, err = c.versionTable()
	if err != nil {
		return Table{}, 0, err
	}
	timeout = c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return versionTable, timeout, nil
}

func (c Config) run(ctx context.Context, operation string, fn func(source.Driver, migratedb.Driver) error) error {
	versionTable, timeout, err := c.settings(true)
	if err != nil {
		return err
	}
	_, err = resilience.Execute(ctx, resilience.NewPolicy().WithTimeout(timeout), func(ctx context.Context) (struct{}, error) {
		return struct{}{}, c.execute(ctx, versionTable, fn)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (c Config) execute(ctx context.Context, versionTable Table, fn func(source.Driver, migratedb.Driver) error) (err error) {
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
	d, err := c.Driver(ctx, pool, versionTable)
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

// maxSourceVersions bounds source enumeration during preflight.
const maxSourceVersions = 10_000

func migrateUp(s source.Driver, d migratedb.Driver, target *uint) error {
	current, err := preflight(s, d, target)
	if err != nil {
		return err
	}
	until := -1
	if target != nil {
		until = int(*target)
	}
	return advance(s, d, current, 1, -1, until)
}

// preflight validates the stored state against the source, and the optional forward target, before any mutation.
func preflight(s source.Driver, d migratedb.Driver, target *uint) (int, error) {
	versions, err := sourceVersions(s)
	if err != nil {
		return 0, err
	}
	current, dirty, err := d.Version()
	if err != nil {
		return 0, err
	}
	if dirty {
		return 0, migrate.ErrDirty{Version: current}
	}
	if current >= 0 && !slices.Contains(versions, uint(current)) {
		return 0, notReady(fmt.Errorf("stored schema version %d is not in the migration source", current))
	}
	if target == nil {
		return current, nil
	}
	if !slices.Contains(versions, *target) {
		return 0, apperrors.InvalidInput("target", "target schema version is not in the migration source")
	}
	if current > int(*target) {
		return 0, notReady(fmt.Errorf("stored schema version %d is ahead of target %d", current, *target))
	}
	for _, version := range versions {
		if int(version) > current && version <= *target {
			if _, err := readMigration(s, version, 1); err != nil {
				return 0, fmt.Errorf("migration %d: %w", version, err)
			}
		}
	}
	return current, nil
}

func notReady(cause error) error {
	return apperrors.New(apperrors.ErrCodeDatabaseError, "database schema is not ready").WithCause(cause)
}

func sourceVersions(s source.Driver) ([]uint, error) {
	version, err := s.First()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	versions := make([]uint, 0, 16)
	for ; err == nil; version, err = s.Next(version) {
		if int(version) < 0 {
			return nil, apperrors.InvalidInput("migration", "schema version exceeds the supported range")
		}
		if len(versions) == maxSourceVersions {
			return nil, apperrors.InvalidInput("migration", "migration source exceeds the version limit")
		}
		versions = append(versions, version)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return versions, nil
}

// advance moves at most count steps (all when negative) in direction, stopping after reaching until when it is not negative.
func advance(s source.Driver, d migratedb.Driver, current, direction, count, until int) error {
	for applied := 0; (count < 0 || applied < count) && (until < 0 || current != until); applied++ {
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
