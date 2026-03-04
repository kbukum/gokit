package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	dberrors "github.com/kbukum/gokit/database/errors"
)

// Repository provides full CRUD operations for any GORM model.
// It embeds WriteRepository, adding Delete.
//
//	type UserRepository struct {
//	    *repository.Repository[models.User, string]
//	}
//
//	func NewUserRepository(db *gorm.DB) *UserRepository {
//	    return &UserRepository{
//	        Repository: repository.NewRepository[models.User, string](db, "user"),
//	    }
//	}
type Repository[T any, ID comparable] struct {
	*WriteRepository[T, ID]
}

// NewRepository creates a generic Repository for model T with ID type ID.
// The resource name is used in error messages (e.g. "user", "bot").
func NewRepository[T any, ID comparable](db *gorm.DB, resource string, opts ...Option) *Repository[T, ID] {
	return &Repository[T, ID]{
		WriteRepository: NewWriteRepository[T, ID](db, resource, opts...),
	}
}

// WithTx returns a new Repository that uses the given transaction.
func (r *Repository[T, ID]) WithTx(tx *gorm.DB) *Repository[T, ID] {
	return &Repository[T, ID]{
		WriteRepository: r.WriteRepository.WithTx(tx),
	}
}

// Delete removes the entity with the given ID.
func (r *Repository[T, ID]) Delete(ctx context.Context, id ID) error {
	var zero T
	if err := r.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", r.idField), id).Delete(&zero).Error; err != nil {
		return dberrors.FromDatabase(err, r.resource)
	}
	return nil
}
