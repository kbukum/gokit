package database

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

// TransactionFunc runs within the caller-owned transaction.
type TransactionFunc func(tx *gorm.DB) error

// WithTransaction commits only a successful, uncanceled callback. Errors and panics roll back; panics continue to the caller.
func (d *DB) WithTransaction(ctx context.Context, fn TransactionFunc) error {
	return d.transaction(ctx, fn, false)
}

// WithReadOnlyTransaction uses the reader pool and always rolls back. Backends supporting read-only transactions also reject writes within the transaction.
func (d *DB) WithReadOnlyTransaction(ctx context.Context, fn TransactionFunc) error {
	return d.transaction(ctx, fn, true)
}

func (d *DB) transaction(ctx context.Context, fn TransactionFunc, readOnly bool) (err error) {
	if fn == nil {
		return apperrors.InvalidInput("transaction", "transaction callback is required")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	session := d.WithContext(ctx)
	if readOnly {
		session = d.ReadOnly(ctx)
	}
	tx := session.Begin(&sql.TxOptions{ReadOnly: readOnly})
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		rbErr := tx.Rollback().Error
		if rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			d.log.ErrorCtx(ctx, "Transaction rollback failed", map[string]any{"error": rbErr.Error()})
			err = errors.Join(err, rbErr)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if readOnly {
		return nil
	}
	return tx.Commit().Error
}
