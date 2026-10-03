package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

// ApplyToGorm applies params to a GORM query and returns a paginated result.
func ApplyToGorm[T any](ctx context.Context, db *gorm.DB, params Params, config Config) (*Result[T], error) {
	params.PageSize = pageSize(params.PageSize, config)
	params.Page = max(params.Page, 1)
	if params.Page-1 > math.MaxInt/params.PageSize {
		return nil, apperrors.InvalidInput("page", "page offset is too large")
	}
	db = db.WithContext(ctx)
	q := db.Session(&gorm.Session{})

	// Free text search
	if params.Query.FreeText != "" && len(config.SearchFields) > 0 {
		q = applySearch(q, params.Query.FreeText, config.SearchFields)
	}

	// Filters
	for _, cond := range params.Query.Conditions {
		q = applyCondition(q, cond, config)
	}

	// Count
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}

	// Facets (cross-filtered against the unfiltered base)
	facets, err := ComputeFacetsWithFilters(ctx, db, config.FacetFields, params.Query.Conditions, config)
	if err != nil {
		return nil, err
	}

	// Sort
	q = applySort(q, params.SortBy, params.SortOrder, config)

	// Paginate
	offset := (params.Page - 1) * params.PageSize
	q = q.Offset(offset).Limit(params.PageSize)

	data := make([]T, 0)
	if err := q.Find(&data).Error; err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	totalPages := int(total / int64(params.PageSize))
	if total%int64(params.PageSize) != 0 {
		totalPages++
	}
	totalPages = max(totalPages, 1)

	return &Result[T]{
		Data: data,
		Pagination: Pagination{
			Page: params.Page, PageSize: params.PageSize,
			Total: int(total), TotalPages: totalPages,
		},
		Facets: facets,
	}, nil
}

func pageSize(size int, config Config) int {
	if size <= 0 {
		return config.defaultPageSize()
	}
	return min(size, config.maxPageSize())
}

func applySearch(db *gorm.DB, search string, fields []string) *gorm.DB {
	pattern := "%" + strings.ToLower(search) + "%"
	conds := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields))
	for _, f := range fields {
		if !isSafeIdentifier(f) {
			return invalidQuery(db, "search", "invalid search field")
		}
		conds = append(conds, fmt.Sprintf("LOWER(%s) LIKE ?", f))
		args = append(args, pattern)
	}
	if len(conds) == 0 {
		return db
	}
	return db.Where(strings.Join(conds, " OR "), args...)
}

func applyCondition(db *gorm.DB, cond Condition, config Config) *gorm.DB {
	field := config.ResolveField(cond.Field)
	if !isSafeIdentifier(field) || !cond.Operator.IsValid() || !isFieldAllowed(cond.Field, config.AllowedFilters) {
		return invalidQuery(db, "filter", "invalid filter field or operator")
	}

	switch cond.Operator {
	case OpEq:
		if len(cond.Values) > 0 {
			return db.Where(fmt.Sprintf("%s IN ?", field), cond.Values)
		}
		return db.Where(fmt.Sprintf("%s = ?", field), cond.Value)
	case OpNeq:
		if len(cond.Values) > 0 {
			return db.Where(fmt.Sprintf("%s NOT IN ?", field), cond.Values)
		}
		return db.Where(fmt.Sprintf("%s != ?", field), cond.Value)
	case OpGt:
		return db.Where(fmt.Sprintf("%s > ?", field), cond.Value)
	case OpGte:
		return db.Where(fmt.Sprintf("%s >= ?", field), cond.Value)
	case OpLt:
		return db.Where(fmt.Sprintf("%s < ?", field), cond.Value)
	case OpLte:
		return db.Where(fmt.Sprintf("%s <= ?", field), cond.Value)
	case OpIn:
		if len(cond.Values) > 0 {
			return db.Where(fmt.Sprintf("%s IN ?", field), cond.Values)
		}
		if cond.Value != "" {
			return db.Where(fmt.Sprintf("%s IN ?", field), strings.Split(cond.Value, ","))
		}
	case OpNin:
		if len(cond.Values) > 0 {
			return db.Where(fmt.Sprintf("%s NOT IN ?", field), cond.Values)
		}
		if cond.Value != "" {
			return db.Where(fmt.Sprintf("%s NOT IN ?", field), strings.Split(cond.Value, ","))
		}
	case OpLike:
		return db.Where(fmt.Sprintf("%s LIKE ?", field), "%"+cond.Value+"%")
	case OpIlike:
		return db.Where(fmt.Sprintf("LOWER(%s) LIKE ?", field), "%"+strings.ToLower(cond.Value)+"%")
	case OpNull:
		return db.Where(fmt.Sprintf("%s IS NULL", field))
	case OpNotNull:
		return db.Where(fmt.Sprintf("%s IS NOT NULL", field))
	}
	return invalidQuery(db, "filter", "filter values are required")
}

func invalidQuery(db *gorm.DB, field, message string) *gorm.DB {
	result := db.Session(&gorm.Session{})
	result.Error = errors.Join(result.Error, apperrors.InvalidInput(field, message))
	return result
}

func applySort(db *gorm.DB, sortBy, sortOrder string, config Config) *gorm.DB {
	if sortBy != "" {
		for _, f := range config.AllowedSortFields {
			if f != sortBy {
				continue
			}
			col := config.ResolveField(sortBy)
			if !isSafeIdentifier(col) {
				return invalidQuery(db, "sort", "invalid sort field")
			}
			order := col
			if sortOrder == "desc" {
				order += " DESC"
			}
			return db.Order(order)
		}
	}
	if config.DefaultSort != "" {
		return db.Order(config.DefaultSort)
	}
	return db
}

// ApplyConditions applies conditions to a GORM query.
func ApplyConditions(db *gorm.DB, conditions []Condition, config Config) *gorm.DB {
	for _, cond := range conditions {
		db = applyCondition(db, cond, config)
	}
	return db
}

// --- Facets ---

// ComputeFacetsWithFilters computes facet counts with cross-filtering.
//
//nolint:nilnil // No configured facets means no optional facet result.
func ComputeFacetsWithFilters(
	ctx context.Context,
	db *gorm.DB,
	facetFields []string,
	conditions []Condition,
	config Config,
) (map[string]map[string]int, error) {
	if len(facetFields) == 0 {
		return nil, nil
	}
	db = db.WithContext(ctx)
	limit := config.MaxFacetValues
	if limit <= 0 || limit > MaxPageSize {
		limit = MaxPageSize
	}

	facets := make(map[string]map[string]int)

	for _, field := range facetFields {
		col := config.ResolveField(field)
		if !isSafeIdentifier(col) {
			return nil, apperrors.InvalidInput("facets", "invalid facet field")
		}
		facetKey := config.ResolveFacetLabel(field)
		facets[facetKey] = make(map[string]int)

		otherConds := excludeFieldConditions(conditions, col, config)
		baseQuery := buildBaseQuery(db, otherConds, config)

		var total int64
		if err := baseQuery.Count(&total).Error; err != nil {
			return nil, fmt.Errorf("facet count: %w", err)
		}
		facets[facetKey]["_total"] = int(total)

		groupQuery := buildBaseQuery(db, otherConds, config)
		type facetCount struct {
			Value string
			Count int
		}
		var counts []facetCount
		if err := groupQuery.Select(fmt.Sprintf("%s as value, COUNT(*) as count", col)).
			Group(col).Limit(limit + 1).Scan(&counts).Error; err != nil {
			return nil, fmt.Errorf("facet query: %w", err)
		}
		if len(counts) > limit {
			return nil, apperrors.InvalidInput("facets", "facet has too many distinct values")
		}

		for _, c := range counts {
			facets[facetKey][c.Value] = c.Count
		}
	}

	return facets, nil
}

func buildBaseQuery(db *gorm.DB, conditions []Condition, config Config) *gorm.DB {
	q := db.Session(&gorm.Session{})
	for _, cond := range conditions {
		q = applyCondition(q, cond, config)
	}
	return q
}

func excludeFieldConditions(conditions []Condition, excludeCol string, config Config) []Condition {
	var result []Condition
	for _, c := range conditions {
		if config.ResolveField(c.Field) != excludeCol {
			result = append(result, c)
		}
	}
	return result
}

// --- Includes ---

// ApplyIncludes applies requested includes to a GORM query using specs from config.
func ApplyIncludes(db *gorm.DB, includes IncludeSet, config IncludeConfig) *gorm.DB {
	if includes.IsEmpty() || len(config.Specs) == 0 {
		return db
	}
	for _, path := range includes.Paths {
		spec, ok := config.Specs[path.Raw]
		if !ok {
			continue
		}
		db = applyIncludeSpec(db, spec)
	}
	return db
}

func applyIncludeSpec(db *gorm.DB, spec IncludeSpec) *gorm.DB {
	if spec.Relation == "" {
		return db
	}
	switch spec.Type {
	case RelationBelongsTo, RelationHasOne:
		return db.Joins(spec.Relation)
	case RelationHasMany:
		return db.Preload(spec.Relation, buildPreloadFunc(spec))
	default:
		return db.Joins(spec.Relation)
	}
}

func buildPreloadFunc(spec IncludeSpec) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		for _, nested := range spec.Nested {
			if nested.Type == RelationBelongsTo || nested.Type == RelationHasOne {
				tx = tx.Joins(nested.Relation)
			}
		}
		if spec.OrderBy != "" {
			tx = tx.Order(spec.OrderBy)
		}
		return tx
	}
}

// ApplyIncludesFromParams is a convenience wrapper.
func ApplyIncludesFromParams(db *gorm.DB, params Params, config Config) *gorm.DB {
	return ApplyIncludes(db, params.Includes, config.IncludeConfig)
}
