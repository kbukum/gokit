package postgres

import (
	"context"
	"regexp"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

var customSetting = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}\.[a-z][a-z0-9_]{0,62}$`)

// SetLocal sets a custom setting, such as one read by row-level security policies through current_setting, for the
// remainder of tx's transaction only. name must be a two-part lowercase custom setting ("app.partition"); value is
// bound. It rejects a handle that is not in a transaction, so a pooled session never retains the value.
//
// Optional row-level security still requires USING and WITH CHECK policies and a non-owner runtime role; a setting
// alone isolates nothing.
func SetLocal(ctx context.Context, tx *gorm.DB, name, value string) error {
	if tx == nil {
		return apperrors.InvalidInput("transaction", "Transaction is required")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return apperrors.InvalidInput("transaction", "Setting requires an active transaction")
	}
	if !customSetting.MatchString(name) {
		return apperrors.InvalidInput("name", "Setting must be a two-part lowercase custom name")
	}
	return tx.WithContext(ctx).Exec("SELECT pg_catalog.set_config(?, ?, true)", name, value).Error
}
