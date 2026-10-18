package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// DefaultSetBudget bounds the migration of one set when [SchemaMigration.SetBudget] is zero.
const DefaultSetBudget = 2 * time.Minute

// SchemaMigration is one schema plan applied with a migration login.
type SchemaMigration struct {
	// Config connects as the migration role. One connection is enough; [ApplySchema] pins one.
	Config database.Config
	Plan   SchemaPlan
	// SetBudget bounds the whole migration per set in Plan; zero means [DefaultSetBudget].
	SetBudget time.Duration
	// Options apply when connecting, such as a secret source for a password file.
	Options []database.Option
}

// Migrate opens a short-lived connection with m.Config, applies m.Plan through [ApplySchema], and closes the
// connection, all within m.SetBudget for each set in the plan.
func Migrate(ctx context.Context, log *logging.Logger, m SchemaMigration) (err error) {
	if util.IsNil(ctx) || log == nil {
		return apperrors.InvalidInput("migration", "Context and logger are required")
	}
	if invalid := m.Plan.Validate(); invalid != nil {
		return invalid
	}
	if m.SetBudget < 0 {
		return apperrors.InvalidInput("set_budget", "must not be negative")
	}
	budget := m.SetBudget
	if budget == 0 {
		budget = DefaultSetBudget
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(max(1, len(m.Plan.Sets)))*budget)
	defer cancel()
	db, err := database.NewWithContext(ctx, Dialect(), m.Config, log, m.Options...)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := ApplySchema(ctx, db.GormDB, m.Plan); err != nil {
		return err
	}
	log.InfoCtx(ctx, "Schema migrated", map[string]any{"schema": m.Plan.Name, "sets": strconv.Itoa(len(m.Plan.Sets))})
	return nil
}
