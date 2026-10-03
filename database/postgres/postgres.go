package postgres

import (
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
)

// Name is the registry key for the PostgreSQL backend.
const Name = "postgres"

// dialect is the PostgreSQL backend. It implements database.StructuredDialect, so a caller may
// supply structured database.ConnParams instead of a hand-written DSN (see dsn.go).
type dialect struct{}

// Dialect returns the PostgreSQL backend dialect.
func Dialect() database.Dialect { return dialect{} }

// Name reports the backend identifier.
func (dialect) Name() string { return Name }

// Open returns a PostgreSQL GORM dialector for the given DSN. It is the low-level primitive; most
// callers select the backend through Dialect or Register instead.
func Open(dsn string) gorm.Dialector {
	return gormpostgres.New(gormpostgres.Config{
		DSN:          dsn,
		OptionOpenDB: []stdlib.OptionOpenDB{stdlib.OptionBeforeConnect(configureConnection)},
	})
}

// Open returns a PostgreSQL GORM dialector for the given DSN.
func (dialect) Open(dsn string) gorm.Dialector { return Open(dsn) }

// Register registers the PostgreSQL dialect in an explicit database registry.
func Register(reg *database.DialectRegistry) error {
	return reg.Register(Dialect())
}
