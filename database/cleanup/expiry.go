package cleanup

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

// Config defines one cleanup batch. ExpiryField names a model field or column; BatchSize defaults to 100 and cannot exceed 1,000.
type Config struct {
	ExpiryField string
	BatchSize   int
	Clock       util.Clock
}

// DeleteExpired removes at most one batch. Use an index on (expiry, primary key), and call again on a later scheduled tick rather than draining an unbounded backlog.
func DeleteExpired[T any](ctx context.Context, db *gorm.DB, cfg Config) (int64, error) {
	if db == nil || cfg.BatchSize < 0 || cfg.BatchSize > 1000 {
		return 0, apperrors.InvalidInput("cleanup", "database and a batch size from 0 to 1000 are required")
	}
	size := cfg.BatchSize
	if size == 0 {
		size = 100
	}
	clock := cfg.Clock
	if clock == nil {
		clock = util.SystemClock{}
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(new(T)); err != nil {
		return 0, err
	}
	expiry := stmt.Schema.LookUpField(cfg.ExpiryField)
	if expiry == nil || expiry.DataType != schema.Time || len(stmt.Schema.PrimaryFields) != 1 {
		return 0, apperrors.InvalidInput("cleanup", "a time-valued expiry field and a single primary key are required")
	}
	key := stmt.Schema.PrimaryFields[0].DBName
	cutoff := clock.Now()
	return resilience.Execute(ctx, resilience.NewPolicy().WithTimeout(30*time.Second), func(ctx context.Context) (int64, error) {
		base := db.WithContext(ctx).Model(new(T))
		expired := clause.Lte{Column: clause.Column{Name: expiry.DBName}, Value: cutoff}
		ids := base.Session(&gorm.Session{}).Select(key).Where(expired).
			Order(clause.OrderByColumn{Column: clause.Column{Name: expiry.DBName}}).
			Order(clause.OrderByColumn{Column: clause.Column{Name: key}}).Limit(size)
		result := base.Session(&gorm.Session{}).Where(clause.Expr{
			SQL: "? IN (?)", Vars: []any{clause.Column{Name: key}, ids},
		}).Where(expired).Delete(new(T))
		return result.RowsAffected, result.Error
	})
}
