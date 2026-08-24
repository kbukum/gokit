package testutil

import (
	"database/sql"
	"errors"
	"io"
	"sync"

	migratedb "github.com/golang-migrate/migrate/v4/database"

	"github.com/kbukum/gokit/database/migration"
)

// MigrationDriver is an in-memory golang-migrate database driver for exercising migration
// orchestration (Up/Down/Steps/Reset/Version) without a real database backend. It records the
// applied version and the number of statements run, and can be told to fail specific operations
// so failure and rollback paths are provable deterministically.
//
// A zero MigrationDriver reports version -1 (no migrations applied); use NewMigrationDriver.
type MigrationDriver struct {
	mu       sync.Mutex
	version  int
	dirty    bool
	runs     int
	failRun  bool
	failSet  bool
	failDrop bool
}

// NewMigrationDriver returns a MigrationDriver with no migrations applied.
func NewMigrationDriver() *MigrationDriver { return &MigrationDriver{version: -1} }

// FailRun makes the next and subsequent Run calls fail, simulating a failing migration statement.
func (d *MigrationDriver) FailRun() *MigrationDriver { d.set(&d.failRun); return d }

// FailSetVersion makes SetVersion calls fail, simulating a schema-version write failure.
func (d *MigrationDriver) FailSetVersion() *MigrationDriver { d.set(&d.failSet); return d }

// FailDrop makes Drop calls fail, simulating a failed schema drop during Reset.
func (d *MigrationDriver) FailDrop() *MigrationDriver { d.set(&d.failDrop); return d }

func (d *MigrationDriver) set(flag *bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	*flag = true
}

// Runs returns how many migration statements have been applied.
func (d *MigrationDriver) Runs() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs
}

// DriverFunc adapts the fake to a migration.DriverFunc, ignoring the *sql.DB it is handed.
func (d *MigrationDriver) DriverFunc() migration.DriverFunc {
	return func(*sql.DB) (migratedb.Driver, error) { return d, nil }
}

// Open implements migratedb.Driver; it returns the receiver unchanged.
func (d *MigrationDriver) Open(string) (migratedb.Driver, error) { return d, nil }

// Close implements migratedb.Driver; the fake holds no resources.
func (d *MigrationDriver) Close() error { return nil }

// Lock implements migratedb.Driver.
func (d *MigrationDriver) Lock() error { return nil }

// Unlock implements migratedb.Driver.
func (d *MigrationDriver) Unlock() error { return nil }

// Run implements migratedb.Driver; it drains the statement and records the run unless FailRun is set.
func (d *MigrationDriver) Run(r io.Reader) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failRun {
		return errors.New("run failed")
	}
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	d.runs++
	return nil
}

// SetVersion implements migratedb.Driver; it records the version unless FailSetVersion is set.
func (d *MigrationDriver) SetVersion(version int, dirty bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failSet {
		return errors.New("set version failed")
	}
	d.version = version
	d.dirty = dirty
	return nil
}

// Version implements migratedb.Driver; it reports the recorded version and dirty flag.
func (d *MigrationDriver) Version() (version int, dirty bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version, d.dirty, nil
}

// Drop implements migratedb.Driver; it clears the recorded version unless FailDrop is set.
func (d *MigrationDriver) Drop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failDrop {
		return errors.New("drop failed")
	}
	d.version = -1
	d.dirty = false
	return nil
}
