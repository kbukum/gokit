package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	dberrors "github.com/kbukum/gokit/database/errors"
	"github.com/kbukum/gokit/database/query"
)

// ReadRepository provides read-only database operations for any GORM model.
// Use it for entities that are generated/immutable and should not be modified.
//
//	type AuditLogRepository struct {
//	    *repository.ReadRepository[models.AuditLog, string]
//	}
type ReadRepository[T any, ID comparable] struct {
	db       *gorm.DB
	resource string
	idField  string
}

// NewReadRepository creates a read-only Repository for model T with ID type ID.
// The resource name is used in error messages (e.g. "audit_log", "report").
func NewReadRepository[T any, ID comparable](db *gorm.DB, resource string, opts ...Option) *ReadRepository[T, ID] {
	cfg := applyOptions(opts)
	return &ReadRepository[T, ID]{
		db:       db,
		resource: resource,
		idField:  cfg.idField,
	}
}

// DB returns the underlying *gorm.DB for custom queries.
func (r *ReadRepository[T, ID]) DB() *gorm.DB {
	return r.db
}

// Resource returns the resource name used in error messages.
func (r *ReadRepository[T, ID]) Resource() string {
	return r.resource
}

// WithTx returns a new ReadRepository that uses the given transaction.
func (r *ReadRepository[T, ID]) WithTx(tx *gorm.DB) *ReadRepository[T, ID] {
	return &ReadRepository[T, ID]{
		db:       tx,
		resource: r.resource,
		idField:  r.idField,
	}
}

// GetByID retrieves a single entity by its primary key.
func (r *ReadRepository[T, ID]) GetByID(ctx context.Context, id ID) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", r.idField), id).First(&entity).Error; err != nil {
		return nil, dberrors.FromDatabase(err, r.resource)
	}
	return &entity, nil
}

// List retrieves entities using the gokit query builder for pagination,
// filtering, sorting, and facets.
func (r *ReadRepository[T, ID]) List(ctx context.Context, params query.Params, config query.Config) (*query.Result[T], error) {
	result, err := query.ApplyToGorm[T](r.db.WithContext(ctx).Model(new(T)), params, config)
	if err != nil {
		return nil, dberrors.FromDatabase(err, r.resource)
	}
	return result, nil
}

// Count returns the total number of (non-deleted) entities.
func (r *ReadRepository[T, ID]) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(new(T)).Count(&count).Error; err != nil {
		return 0, dberrors.FromDatabase(err, r.resource)
	}
	return count, nil
}

// FindOneBy retrieves a single entity matching field == value.
func (r *ReadRepository[T, ID]) FindOneBy(ctx context.Context, field string, value any) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", field), value).First(&entity).Error; err != nil {
		return nil, dberrors.FromDatabase(err, r.resource)
	}
	return &entity, nil
}

// FindAllBy retrieves all entities matching field == value.
func (r *ReadRepository[T, ID]) FindAllBy(ctx context.Context, field string, value any) ([]T, error) {
	var entities []T
	if err := r.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", field), value).Find(&entities).Error; err != nil {
		return nil, dberrors.FromDatabase(err, r.resource)
	}
	return entities, nil
}
