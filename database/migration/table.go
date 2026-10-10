package migration

import (
	"io/fs"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// Table is a migration metadata identity. Schema is required by PostgreSQL; SQLite supports only its main schema.
type Table struct {
	Schema string
	Name   string
}

// Validate checks both identifier boundaries before any SQL interpolation.
func (t Table) Validate() error {
	if !IsVersionTable(t.Name) {
		return apperrors.InvalidInput("version_table", "version table must be a lowercase identifier of at most 63 characters ending in schema_migrations")
	}
	if t.Schema != "" && !IsIdentifier(t.Schema) {
		return apperrors.InvalidInput("schema", "schema must be a lowercase identifier of at most 63 characters")
	}
	return nil
}

// SQL returns the separately quoted identity. Call Validate before using it in a statement.
func (t Table) SQL() string {
	name := `"` + strings.ReplaceAll(t.Name, `"`, `""`) + `"`
	if t.Schema == "" {
		return name
	}
	return `"` + strings.ReplaceAll(t.Schema, `"`, `""`) + `".` + name
}

// Set describes one independently versioned source and its required target.
type Set struct {
	FS              fs.FS
	Path            string
	Table           Table
	ExpectedVersion uint
}
