package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Name is the registry key for the PostgreSQL backend.
const Name = "postgres"

// dialect is the PostgreSQL backend with controlled structured connection preparation.
type dialect struct{}

// Dialect returns the PostgreSQL backend dialect.
func Dialect() database.Dialect { return dialect{} }

// Name reports the backend identifier.
func (dialect) Name() string { return Name }

type opener struct {
	config   *pgx.ConnConfig
	location *time.Location
}

type ownedDialector struct {
	*gormpostgres.Dialector
	pool *sql.DB
}

func (dialect) Prepare(ctx context.Context, input database.ConnectionInput) (database.Opener, error) {
	return Prepare(ctx, input)
}

func (o *opener) Open(ctx context.Context) (gorm.Dialector, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pool := stdlib.OpenDB(*o.config.Copy(), configureConnection(o.location))
	return &ownedDialector{
		Dialector: &gormpostgres.Dialector{Config: &gormpostgres.Config{Conn: pool}},
		pool:      pool,
	}, nil
}

func (d *ownedDialector) Close() error { return d.pool.Close() }

// Register registers the PostgreSQL dialect in an explicit database registry.
func Register(reg *database.DialectRegistry) error {
	return reg.Register(Dialect())
}
