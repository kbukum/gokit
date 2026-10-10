package sqlite

import (
	"context"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Name is the registry key for the SQLite backend.
const Name = "sqlite"

// dialect is the SQLite backend; its opaque configuration is a filename or memory URI.
type dialect struct{}

// Dialect returns the SQLite backend dialect.
func Dialect() database.Dialect { return dialect{} }

// Name reports the backend identifier.
func (dialect) Name() string { return Name }

type opener struct{ dsn string }

// Prepare validates SQLite configuration without allocating a pool.
func Prepare(ctx context.Context, input database.ConnectionInput) (database.Opener, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if input.DSN == "" {
		return nil, apperrors.InvalidInput("connection", "SQLite requires an opaque filename or memory URI")
	}
	_, query, _ := strings.Cut(input.DSN, "?")
	if _, err := url.ParseQuery(query); err != nil {
		return nil, apperrors.InvalidInput("dsn", "Invalid SQLite connection options").WithCause(database.Failure(err))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return opener{dsn: input.DSN}, nil
}

func (dialect) Prepare(ctx context.Context, input database.ConnectionInput) (database.Opener, error) {
	return Prepare(ctx, input)
}

func (o opener) Open(ctx context.Context) (gorm.Dialector, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("context", "Context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &dialector{dsn: o.dsn}, nil
}

// Register registers the SQLite dialect in an explicit database registry.
func Register(reg *database.DialectRegistry) error {
	return reg.Register(Dialect())
}
