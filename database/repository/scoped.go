package repository

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	dberrors "github.com/kbukum/gokit/database/errors"
	"github.com/kbukum/gokit/database/query"
	apperrors "github.com/kbukum/gokit/errors"
)

// ScopedConfig configures a partition-enforced repository for scalar row type T and scope S. The repository is
// product-neutral: S is any caller type, such as an organization/project pair, that BindScope turns into one SQL scalar
// per immutable partition column.
type ScopedConfig[T, S any] struct {
	// Resource names the row kind in errors.
	Resource string
	// IDColumn is the primary-key column. It defaults to "id".
	IDColumn string
	// ScopeColumns lists the immutable partition columns bound on every operation. GORM must be allowed to create them.
	ScopeColumns []string
	// BindScope returns exactly one non-nil, non-empty scalar per ScopeColumns entry, in order.
	BindScope func(S) ([]any, error)
	// MutableColumns lists the only columns Update writes. GORM must be allowed to update them. Identity and partition
	// columns are never mutable.
	MutableColumns []string
	// Query restricts List filtering, search and sorting. Facets and includes are unsupported.
	Query query.Config
}

// ScopedSpec is an immutable, prevalidated scoped repository definition. Validate it once with [NewScopedSpec] and
// bind it cheaply to a pool or transaction with [ScopedSpec.Bind].
//
// Rows are scalar: models with relationships are rejected, hooks never run and associations are never persisted.
// Every operation binds all partition columns, so a row outside the scope is indistinguishable from a missing row.
// Trusted custom SQL in a persistence adapter is not sandboxed by this type.
type ScopedSpec[T any, ID comparable, S any] struct {
	resource  string
	id        *schema.Field
	scope     []*schema.Field
	bindScope func(S) ([]any, error)
	mutable   []*schema.Field
	query     query.Config
}

// NewScopedSpec validates cfg against T's GORM metadata using GORM's default naming strategy.
func NewScopedSpec[T any, ID comparable, S any](cfg ScopedConfig[T, S]) (*ScopedSpec[T, ID, S], error) {
	if cfg.Resource == "" {
		return nil, invalid("resource", "Scoped repository resource is required")
	}
	if cfg.BindScope == nil {
		return nil, invalid("scope", "Scoped repository scope binder is required")
	}
	if len(cfg.ScopeColumns) == 0 {
		return nil, invalid("scope", "Scoped repository requires at least one partition column")
	}
	model := reflect.TypeFor[T]()
	if model.Kind() != reflect.Struct {
		return nil, invalid("model", "Scoped repository rows must be structs")
	}
	parsed, err := schema.Parse(new(T), &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return nil, apperrors.InvalidInput("model", "Scoped repository model is not a GORM model").WithCause(err)
	}
	if len(parsed.Relationships.Relations) > 0 || len(parsed.Relationships.EmbeddedRelations) > 0 {
		return nil, invalid("model", "Scoped repository rows must not declare associations")
	}
	idColumn := cfg.IDColumn
	if idColumn == "" {
		idColumn = "id"
	}
	spec := &ScopedSpec[T, ID, S]{resource: cfg.Resource, bindScope: cfg.BindScope, query: cfg.Query}
	if spec.id = parsed.LookUpField(idColumn); spec.id == nil || !spec.id.PrimaryKey || spec.id.DBName != idColumn {
		return nil, invalid("id", "Scoped repository ID column must be a primary-key column")
	}
	if spec.id.FieldType != reflect.TypeFor[ID]() {
		return nil, invalid("id", "Scoped repository ID type does not match the model")
	}
	seen := map[string]bool{idColumn: true}
	if spec.scope, err = columns(parsed, cfg.ScopeColumns, seen, scopeWritable); err != nil {
		return nil, err
	}
	if spec.mutable, err = columns(parsed, cfg.MutableColumns, seen, mutableWritable); err != nil {
		return nil, err
	}
	if err := validateQuery(parsed, cfg.Query); err != nil {
		return nil, err
	}
	return spec, nil
}

// scopeWritable requires GORM to insert the stamped partition value; a non-creatable field would let a database
// default decide the partition. mutableWritable requires Update to write the column instead of silently skipping it.
func scopeWritable(field *schema.Field) bool   { return field.Creatable }
func mutableWritable(field *schema.Field) bool { return field.Updatable }

func columns(parsed *schema.Schema, names []string, seen map[string]bool, writable func(*schema.Field) bool) ([]*schema.Field, error) {
	fields := make([]*schema.Field, 0, len(names))
	for _, name := range names {
		field := parsed.LookUpField(name)
		if field == nil || field.DBName != name || seen[name] {
			return nil, invalid("columns", fmt.Sprintf("Scoped repository column %q is unknown, repeated or reserved", name))
		}
		if !writable(field) {
			return nil, invalid("columns", fmt.Sprintf("Scoped repository column %q has GORM write permissions that prevent enforcement", name))
		}
		seen[name] = true
		fields = append(fields, field)
	}
	return fields, nil
}

func validateQuery(parsed *schema.Schema, cfg query.Config) error {
	if len(cfg.FacetFields) > 0 || len(cfg.IncludeConfig.AllowedPaths) > 0 || len(cfg.IncludeConfig.Specs) > 0 {
		return invalid("query", "Scoped repositories do not support facets or includes")
	}
	names := slices.Concat(cfg.SearchFields, cfg.AllowedSortFields, cfg.AllowedFilters)
	if cfg.DefaultSort != "" {
		names = append(names, cfg.DefaultSort)
	}
	for _, name := range names {
		if field := parsed.LookUpField(cfg.ResolveField(name)); field == nil || field.DBName != cfg.ResolveField(name) {
			return invalid("query", fmt.Sprintf("Scoped repository query field %q is not a model column", name))
		}
	}
	return nil
}

// Bind returns a repository over db, which may be a pool or a transaction. Prior statement clauses on db are discarded
// while its connection or transaction is kept.
func (s *ScopedSpec[T, ID, S]) Bind(db *gorm.DB) (*ScopedRepository[T, ID, S], error) {
	if s == nil || db == nil {
		return nil, invalid("database", "Scoped repository requires a specification and database")
	}
	return &ScopedRepository[T, ID, S]{spec: s, db: db.Session(&gorm.Session{NewDB: true, SkipHooks: true})}, nil
}

// ScopedRepository performs partition-enforced Get, List, Create, Update and Delete on scalar rows.
type ScopedRepository[T any, ID comparable, S any] struct {
	spec *ScopedSpec[T, ID, S]
	db   *gorm.DB
}

// Get returns the row with id inside scope, or the same NOT_FOUND error for a missing or out-of-scope row.
func (r *ScopedRepository[T, ID, S]) Get(ctx context.Context, scope S, id ID) (T, error) {
	var row T
	where, err := r.where(scope, &id)
	if err != nil {
		return row, err
	}
	if err := r.statement(ctx).Where(where).Take(&row).Error; err != nil {
		return row, r.failure(err)
	}
	return row, nil
}

// List returns one bounded page of rows inside scope. The partition conjunction applies to the count and the page,
// including free-text OR search.
func (r *ScopedRepository[T, ID, S]) List(ctx context.Context, scope S, params query.Params) (*query.Result[T], error) {
	if !params.Includes.IsEmpty() {
		return nil, invalid("includes", "Scoped repositories do not support includes")
	}
	where, err := r.where(scope, nil)
	if err != nil {
		return nil, err
	}
	result, err := query.ApplyToGorm[T](ctx, r.statement(ctx).Model(new(T)).Where(where), params, r.spec.query)
	if err != nil {
		return nil, r.failure(err)
	}
	return result, nil
}

// Create inserts a copy of row stamped with the bound partition values and returns it with generated values. A row
// already carrying a different partition value is rejected; row is never mutated.
func (r *ScopedRepository[T, ID, S]) Create(ctx context.Context, scope S, row T) (T, error) {
	var zero T
	values, err := r.scopeValues(scope)
	if err != nil {
		return zero, err
	}
	created := row
	target := reflect.ValueOf(&created).Elem()
	for i, field := range r.spec.scope {
		if err := conflicts(ctx, field, target, values[i], "Row partition does not match the bound scope"); err != nil {
			return zero, err
		}
		if err := field.Set(ctx, target, values[i]); err != nil {
			return zero, apperrors.InvalidInput("scope", "Scope value does not fit the partition column").WithCause(err)
		}
	}
	if err := r.statement(ctx).Omit(clause.Associations).Create(&created).Error; err != nil {
		return zero, r.failure(err)
	}
	return created, nil
}

// Update writes every mutable column of row, including zero values, to the row with id inside scope. It never
// inserts; zero matched rows return NOT_FOUND. row's identity and partition fields must be zero or equal the bound
// values.
func (r *ScopedRepository[T, ID, S]) Update(ctx context.Context, scope S, id ID, row T) error {
	if len(r.spec.mutable) == 0 {
		return invalid("columns", "Scoped repository declares no mutable columns")
	}
	values, err := r.scopeValues(scope)
	if err != nil {
		return err
	}
	source := reflect.ValueOf(&row).Elem()
	if err := conflicts(ctx, r.spec.id, source, id, "Row identity cannot be updated"); err != nil {
		return err
	}
	for i, field := range r.spec.scope {
		if err := conflicts(ctx, field, source, values[i], "Row partition cannot be updated"); err != nil {
			return err
		}
	}
	changes := make(map[string]any, len(r.spec.mutable))
	for _, field := range r.spec.mutable {
		changes[field.DBName], _ = field.ValueOf(ctx, source)
	}
	result := r.statement(ctx).Model(new(T)).Where(r.conditions(values, &id)).Updates(changes)
	return r.affected(result)
}

// Delete removes the row with id inside scope; zero matched rows return NOT_FOUND.
func (r *ScopedRepository[T, ID, S]) Delete(ctx context.Context, scope S, id ID) error {
	where, err := r.where(scope, &id)
	if err != nil {
		return err
	}
	return r.affected(r.statement(ctx).Where(where).Delete(new(T)))
}

func (r *ScopedRepository[T, ID, S]) statement(ctx context.Context) *gorm.DB {
	return r.db.Session(&gorm.Session{NewDB: true, SkipHooks: true, Context: ctx})
}

func (r *ScopedRepository[T, ID, S]) where(scope S, id *ID) (clause.Expression, error) {
	values, err := r.scopeValues(scope)
	if err != nil {
		return nil, err
	}
	return r.conditions(values, id), nil
}

func (r *ScopedRepository[T, ID, S]) conditions(values []any, id *ID) clause.Expression {
	exprs := make([]clause.Expression, 0, len(values)+1)
	for i, field := range r.spec.scope {
		exprs = append(exprs, clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: field.DBName}, Value: values[i]})
	}
	if id != nil {
		exprs = append(exprs, clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: r.spec.id.DBName}, Value: *id})
	}
	return clause.And(exprs...)
}

func (r *ScopedRepository[T, ID, S]) scopeValues(scope S) ([]any, error) {
	values, err := r.spec.bindScope(scope)
	if err != nil {
		return nil, apperrors.InvalidInput("scope", "Scope is invalid").WithCause(err)
	}
	if len(values) != len(r.spec.scope) {
		return nil, invalid("scope", "Scope binder returned the wrong number of partition values")
	}
	owned := make([]any, len(values))
	for i, value := range values {
		if owned[i], err = scalar(value); err != nil {
			return nil, err
		}
	}
	return owned, nil
}

// scalar accepts one non-empty SQL scalar and copies retained byte slices.
func scalar(value any) (any, error) {
	if valuer, ok := value.(driver.Valuer); ok {
		if v := reflect.ValueOf(value); v.Kind() == reflect.Pointer && v.IsNil() {
			return nil, invalid("scope", "Partition value is required")
		}
		resolved, err := valuer.Value()
		if err != nil {
			return nil, apperrors.InvalidInput("scope", "Partition value is invalid").WithCause(err)
		}
		if _, err := scalar(resolved); err != nil {
			return nil, err
		}
		return value, nil
	}
	v := reflect.ValueOf(value)
	switch {
	case !v.IsValid():
		return nil, invalid("scope", "Partition value is required")
	case v.Kind() == reflect.String && v.Len() > 0,
		v.CanInt(), v.CanUint(),
		v.Kind() == reflect.Array && v.Type().Elem().Kind() == reflect.Uint8:
		return value, nil
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 && v.Len() > 0:
		return slices.Clone(v.Bytes()), nil
	default:
		return nil, invalid("scope", "Partition value must be a non-empty scalar")
	}
}

// conflicts rejects a non-zero row field that differs from bound after converting bound to the field's type.
func conflicts(ctx context.Context, field *schema.Field, row reflect.Value, bound any, message string) error {
	current, isZero := field.ValueOf(ctx, row)
	if isZero {
		return nil
	}
	scratch := reflect.New(row.Type()).Elem()
	if err := field.Set(ctx, scratch, bound); err != nil {
		return apperrors.InvalidInput(field.DBName, "Bound value does not fit the column").WithCause(err)
	}
	if expected, _ := field.ValueOf(ctx, scratch); !reflect.DeepEqual(current, expected) {
		return invalid(field.DBName, message)
	}
	return nil
}

func (r *ScopedRepository[T, ID, S]) affected(result *gorm.DB) error {
	if result.Error != nil {
		return r.failure(result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.NotFound(r.spec.resource, "")
	}
	return nil
}

// failure keeps typed application errors, such as rejected queries, and classifies driver errors with their causes.
func (r *ScopedRepository[T, ID, S]) failure(err error) error {
	return dberrors.FromDatabase(err, r.spec.resource)
}

func invalid(field, message string) *apperrors.AppError {
	return apperrors.InvalidInput(field, message)
}
