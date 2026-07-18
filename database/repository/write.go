package repository

import (
	"context"

	"gorm.io/gorm"

	dberrors "github.com/kbukum/gokit/database/errors"
)

// WriteRepository provides read and write (Create/Update) operations.
// Use it for entities that can be created and modified but not deleted.
//
//	type LedgerRepository struct {
//	    *repository.WriteRepository[models.LedgerEntry, string]
//	}
type WriteRepository[T any, ID comparable] struct {
	*ReadRepository[T, ID]
}

// NewWriteRepository creates a WriteRepository for model T with ID type ID.
// The resource name is used in error messages (e.g. "ledger_entry", "event").
func NewWriteRepository[T any, ID comparable](db *gorm.DB, resource string, opts ...Option) *WriteRepository[T, ID] {
	return &WriteRepository[T, ID]{
		ReadRepository: NewReadRepository[T, ID](db, resource, opts...),
	}
}

// WithTx returns a new WriteRepository that uses the given transaction.
func (r *WriteRepository[T, ID]) WithTx(tx *gorm.DB) *WriteRepository[T, ID] {
	return &WriteRepository[T, ID]{
		ReadRepository: r.ReadRepository.WithTx(tx),
	}
}

// Create inserts entity into the database.
func (r *WriteRepository[T, ID]) Create(ctx context.Context, entity *T) error {
	if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
		return dberrors.FromDatabase(err, r.resource)
	}
	return nil
}

// Update saves the entity (full update).
func (r *WriteRepository[T, ID]) Update(ctx context.Context, entity *T) error {
	if err := r.db.WithContext(ctx).Save(entity).Error; err != nil {
		return dberrors.FromDatabase(err, r.resource)
	}
	return nil
}
