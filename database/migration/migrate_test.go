package migration

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4/database"
	"gorm.io/gorm"
)

func TestConfigMethodsFailClosedOnZeroValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(Config) error
	}{
		{"Up", func(c Config) error { return c.Up(context.Background()) }},
		{"Down", func(c Config) error { return c.Down(context.Background()) }},
		{"Steps", func(c Config) error { return c.Steps(context.Background(), 1) }},
		{"Reset", func(c Config) error { return c.Reset(context.Background()) }},
		{"Version", func(c Config) error { _, _, err := c.Version(context.Background()); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(Config{})
			if err == nil {
				t.Fatal("expected error for zero-valued Config, got nil")
			}
			if !strings.Contains(err.Error(), "database, source, path and driver are required") {
				t.Fatalf("expected DB-required error, got %v", err)
			}
		})
	}
}

func TestConfigRequiresDriver(t *testing.T) {
	t.Parallel()
	c := Config{DB: &gorm.DB{}}
	err := c.Up(context.Background())
	if err == nil {
		t.Fatal("expected error when Driver is missing, got nil")
	}
	if !strings.Contains(err.Error(), "database, source, path and driver are required") {
		t.Fatalf("expected Driver-required error, got %v", err)
	}
}

func TestConfigRequiresPath(t *testing.T) {
	t.Parallel()
	c := Config{
		DB:     &gorm.DB{},
		Driver: func(context.Context, *sql.DB, string) (database.Driver, error) { return nil, errors.New("unused") },
	}
	err := c.Up(context.Background())
	if err == nil {
		t.Fatal("expected error when Path is missing, got nil")
	}
	if !strings.Contains(err.Error(), "database, source, path and driver are required") {
		t.Fatalf("expected Path-required error, got %v", err)
	}
}

func TestVersionTable(t *testing.T) {
	t.Parallel()
	valid := []string{DefaultVersionTable, "auth_session_schema_migrations", "a_schema_migrations"}
	invalid := []string{"", "_schema_migrations", "app_items", "xschema_migrations", "Auth_schema_migrations", `a"_schema_migrations`, "a;drop_schema_migrations", strings.Repeat("a", 46) + "_schema_migrations"}
	for _, name := range valid {
		if !IsVersionTable(name) {
			t.Errorf("IsVersionTable(%q) = false", name)
		}
	}
	unused := func(context.Context, *sql.DB, string) (database.Driver, error) { return nil, errors.New("unused") }
	for _, name := range invalid {
		if IsVersionTable(name) && name != "" {
			t.Errorf("IsVersionTable(%q) = true", name)
		}
		if name == "" {
			continue
		}
		c := Config{DB: &gorm.DB{}, FS: fstest.MapFS{}, Path: "m", Driver: unused, VersionTable: name}
		if err := c.Up(context.Background()); err == nil || !strings.Contains(err.Error(), "version table") {
			t.Errorf("VersionTable %q accepted: %v", name, err)
		}
	}
	if _, err := NewSQLDriver(context.Background(), &sql.DB{}, nopBackend{}, "bad"); err == nil {
		t.Error("NewSQLDriver accepted an invalid version table")
	}
}

type nopBackend struct{}

func (nopBackend) Lock(context.Context, *sql.Conn) error   { return nil }
func (nopBackend) Unlock(context.Context, *sql.Conn) error { return nil }
func (nopBackend) Drop(context.Context, *sql.Tx) error     { return nil }
