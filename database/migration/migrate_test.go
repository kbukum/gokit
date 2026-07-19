package migration

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4/database"
	"gorm.io/gorm"
)

func TestConfigMethodsFailClosedOnZeroValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(Config) error
	}{
		{"Up", func(c Config) error { return c.Up() }},
		{"Down", func(c Config) error { return c.Down() }},
		{"Steps", func(c Config) error { return c.Steps(1) }},
		{"Reset", func(c Config) error { return c.Reset() }},
		{"Version", func(c Config) error { _, _, err := c.Version(); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(Config{})
			if err == nil {
				t.Fatal("expected error for zero-valued Config, got nil")
			}
			if !strings.Contains(err.Error(), "Config.DB is required") {
				t.Fatalf("expected DB-required error, got %v", err)
			}
		})
	}
}

func TestConfigRequiresDriver(t *testing.T) {
	t.Parallel()
	c := Config{DB: &gorm.DB{}}
	err := c.Up()
	if err == nil {
		t.Fatal("expected error when Driver is missing, got nil")
	}
	if !strings.Contains(err.Error(), "Config.Driver is required") {
		t.Fatalf("expected Driver-required error, got %v", err)
	}
}

func TestConfigRequiresPath(t *testing.T) {
	t.Parallel()
	c := Config{
		DB:     &gorm.DB{},
		Driver: func(*sql.DB) (database.Driver, error) { return nil, errors.New("unused") },
	}
	err := c.Up()
	if err == nil {
		t.Fatal("expected error when Path is missing, got nil")
	}
	if !strings.Contains(err.Error(), "Config.Path is required") {
		t.Fatalf("expected Path-required error, got %v", err)
	}
}
