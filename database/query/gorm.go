package query

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ApplyToGorm applies params to a GORM query and returns a paginated result.
func ApplyToGorm[T any](db *gorm.DB, params Params, config Config) (*Result[T], error) {
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
	facets := ComputeFacetsWithFilters(db, config.FacetFields, params.Query.Conditions, config)

	// Sort
	q = applySort(q, params.SortBy, params.SortOrder, config)

	// Paginate
	if !params.NoPagination {
		offset := (params.Page - 1) * params.PageSize
		q = q.Offset(offset).Limit(params.PageSize)
	}

	var data []T
	if err := q.Find(&data).Error; err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	var totalPages, pageSize int
	if params.NoPagination {
		totalPages = 1
		pageSize = int(total)
	} else {
		totalPages = (int(total) + params.PageSize - 1) / params.PageSize
		if totalPages < 1 {
			totalPages = 1
		}
		pageSize = params.PageSize
	}

	return &Result[T]{
		Data: data,
		Pagination: Pagination{
			Page: params.Page, PageSize: pageSize,
			Total: int(total), TotalPages: totalPages,
		},
		Facets: facets,
	}, nil
}

func applySearch(db *gorm.DB, search string, fields []string) *gorm.DB {
	pattern := "%" + strings.ToLower(search) + "%"
	conds := make([]string, 0, len(fields))
	args := make([]interface{}, 0, len(fields))
	for _, f := range fields {
		conds = append(conds, fmt.Sprintf("LOWER(%s) LIKE ?", f))
		args = append(args, pattern)
	}
	return db.Where(strings.Join(conds, " OR "), args...)
}

func applyCondition(db *gorm.DB, cond Condition, config Config) *gorm.DB {
	field := config.ResolveField(cond.Field)

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
	return db
}

func applySort(db *gorm.DB, sortBy, sortOrder string, config Config) *gorm.DB {
	if sortBy != "" {
		for _, f := range config.AllowedSortFields {
			if f == sortBy {
				order := config.ResolveField(sortBy)
				if sortOrder == "desc" {
					order += " DESC"
				}
				return db.Order(order)
			}
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
func ComputeFacetsWithFilters(
	db *gorm.DB,
	facetFields []string,
	conditions []Condition,
	config Config,
) map[string]map[string]int {
	if len(facetFields) == 0 {
		return nil
	}

	facets := make(map[string]map[string]int)

	for _, field := range facetFields {
		facetKey := config.ResolveFacetLabel(field)
		facets[facetKey] = make(map[string]int)

		otherConds := excludeFieldConditions(conditions, field, config)
		baseQuery := buildBaseQuery(db, otherConds, config)

		var total int64
		baseQuery.Count(&total)
		facets[facetKey]["_total"] = int(total)

		groupQuery := buildBaseQuery(db, otherConds, config)
		type facetCount struct {
			Value string
			Count int
		}
		var counts []facetCount
		groupQuery.Select(fmt.Sprintf("%s as value, COUNT(*) as count", field)).
			Group(field).Scan(&counts)

		for _, c := range counts {
			facets[facetKey][c.Value] = c.Count
		}
	}

	return facets
}

func buildBaseQuery(db *gorm.DB, conditions []Condition, config Config) *gorm.DB {
	q := db.Session(&gorm.Session{})
	for _, cond := range conditions {
		q = applyCondition(q, cond, config)
	}
	return q
}

func excludeFieldConditions(conditions []Condition, excludeField string, config Config) []Condition {
	var result []Condition
	for _, c := range conditions {
		if config.ResolveField(c.Field) != excludeField {
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
