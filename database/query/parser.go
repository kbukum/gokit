package query

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ParseFromRequest extracts query params from an HTTP request. Supports PostgREST/Supabase filter format: field=op.value Accepts page size from: pageSize, page_size, per_page, or limit (in priority order).
func ParseFromRequest(r *http.Request, config Config) Params {
	q := r.URL.Query()

	defPS := config.defaultPageSize()
	maxPS := config.maxPageSize()

	limitStr := q.Get("limit")
	noPagination := limitStr == "-1" || limitStr == "all"

	params := Params{
		Page:         intOrDefault(q.Get("page"), 1),
		PageSize:     clamp(intOrDefault(limitStr, defPS), 1, maxPS),
		NoPagination: noPagination,
		SortBy:       q.Get("sortBy"),
		SortOrder:    normalizeSortOrder(q.Get("order")),
		Query: FilterQuery{
			Conditions: []Condition{},
			FreeText:   strings.TrimSpace(q.Get("search")),
		},
	}

	// Check page size params in priority order: pageSize > page_size > per_page
	for _, key := range []string{"pageSize", "page_size", "per_page"} {
		ps := q.Get(key)
		if ps == "" {
			continue
		}
		if ps == "-1" || ps == "all" {
			params.NoPagination = true
		} else {
			params.PageSize = clamp(intOrDefault(ps, defPS), 1, maxPS)
		}
		break
	}

	if filterStr := q.Get("filter"); filterStr != "" {
		params.Query.Conditions = append(params.Query.Conditions,
			parseFilterString(filterStr, config.AllowedFilters)...)
	}

	for _, field := range config.AllowedFilters {
		if v := q.Get(field); v != "" {
			if cond := parseCondition(field, v); cond != nil {
				params.Query.Conditions = append(params.Query.Conditions, *cond)
			}
		}
	}

	params.Includes = ParseIncludesFromRequest(r, config.IncludeConfig)
	return params
}

// parseFilterString parses "status=eq.active&priority=gt.3".
func parseFilterString(filterStr string, allowedFields []string) []Condition {
	var conditions []Condition
	for _, part := range strings.Split(filterStr, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		field, value := kv[0], kv[1]
		if !isFieldAllowed(field, allowedFields) {
			continue
		}
		if cond := parseCondition(field, value); cond != nil {
			conditions = append(conditions, *cond)
		}
	}
	return conditions
}

// parseCondition parses a single PostgREST-style condition (op.value).
func parseCondition(field, value string) *Condition {
	if value == "is.null" {
		return &Condition{Field: field, Operator: OpNull}
	}
	if value == "not.is.null" {
		return &Condition{Field: field, Operator: OpNotNull}
	}

	dotIdx := strings.Index(value, ".")
	if dotIdx == -1 {
		return &Condition{Field: field, Operator: OpEq, Value: value}
	}

	op := Operator(value[:dotIdx])
	rawValue := value[dotIdx+1:]
	if !op.IsValid() {
		return &Condition{Field: field, Operator: OpEq, Value: value}
	}

	if strings.HasPrefix(rawValue, "(") && strings.HasSuffix(rawValue, ")") {
		return &Condition{Field: field, Operator: op, Values: parseArrayValues(rawValue[1 : len(rawValue)-1])}
	}

	return &Condition{Field: field, Operator: op, Value: unescapeValue(rawValue)}
}

func parseArrayValues(inner string) []string {
	var values []string
	var current strings.Builder
	escaped := false
	for _, ch := range inner {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ',' {
			if s := strings.TrimSpace(current.String()); s != "" {
				values = append(values, s)
			}
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		values = append(values, s)
	}
	return values
}

func unescapeValue(s string) string {
	var result strings.Builder
	escaped := false
	for _, ch := range s {
		if escaped {
			result.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func isFieldAllowed(field string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, f := range allowed {
		if f == field {
			return true
		}
	}
	return false
}

func intOrDefault(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}

// clamp confines v to the inclusive range [lower, upper]. The lower bound is passed even when callers currently always supply 1 so the helper stays generic for future call sites that need a different minimum.
//
//nolint:unparam // see godoc — lower kept for API generality
func clamp(v, lower, upper int) int {
	if v < lower {
		return lower
	}
	if v > upper {
		return upper
	}
	return v
}

func normalizeSortOrder(s string) string {
	if strings.EqualFold(s, "desc") {
		return "desc"
	}
	return "asc"
}
