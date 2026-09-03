package sqlite

import (
	"github.com/kbukum/gokit/database"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Name is the registry key for the SQLite backend.
const Name = "sqlite"

// dialect is the SQLite backend. SQLite has no structured DSN form — its DSN is a file path or
// ":memory:" — so it implements only database.Dialect (not StructuredDialect); callers set
// Config.DSN directly.
type dialect struct{}

// Dialect returns the SQLite backend dialect.
func Dialect() database.Dialect { return dialect{} }

// Name reports the backend identifier.
func (dialect) Name() string { return Name }

// Open returns a SQLite GORM dialector for the given DSN. It is the low-level primitive; most
// callers select the backend through Dialect or Register instead.
func Open(dsn string) gorm.Dialector { return gormsqlite.Open(dsn) }

// Open returns a SQLite GORM dialector for the given DSN.
func (dialect) Open(dsn string) gorm.Dialector { return Open(dsn) }

// Register registers the SQLite dialect in an explicit database registry.
func Register(reg *database.DialectRegistry) error {
	return reg.Register(Dialect())
}
