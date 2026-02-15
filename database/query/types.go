// Package query provides a PostgREST-style query builder for GORM with
// pagination, sorting, filtering, facets, and eager-loading support.
package query

import (
	"net/http"
	"strings"
)

// Operator represents filter operators matching PostgREST/Supabase format.
type Operator string

const (
	OpEq      Operator = "eq"
	OpNeq     Operator = "neq"
	OpGt      Operator = "gt"
	OpGte     Operator = "gte"
	OpLt      Operator = "lt"
	OpLte     Operator = "lte"
	OpIn      Operator = "in"
	OpNin     Operator = "nin"
	OpLike    Operator = "like"
	OpIlike   Operator = "ilike"
	OpNull    Operator = "null"
	OpNotNull Operator = "notNull"
)

// AllOperators returns all valid operators.
func AllOperators() []Operator {
	return []Operator{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIn, OpNin, OpLike, OpIlike, OpNull, OpNotNull}
}

// IsValid reports whether the operator is known.
func (o Operator) IsValid() bool {
	for _, v := range AllOperators() {
		if o == v {
			return true
		}
	}
	return false
}

// Condition represents a single filter condition.
type Condition struct {
	Field    string
	Operator Operator
	Value    string
	Values   []string // for in, nin
}

// FilterQuery holds parsed filter conditions.
type FilterQuery struct {
	Conditions []Condition
	FreeText   string
}

// Params holds parsed query parameters.
type Params struct {
	Page         int
	PageSize     int
	NoPagination bool
	SortBy       string
	SortOrder    string
	Query        FilterQuery
	Includes     IncludeSet
}

// AddCondition appends a condition to the query.
func (p *Params) AddCondition(field string, op Operator, value string) {
	p.Query.Conditions = append(p.Query.Conditions, Condition{
		Field: field, Operator: op, Value: value,
	})
}

// Pagination metadata returned in paginated results.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// Result is a paginated response with optional facets.
type Result[T any] struct {
	Data       []T                       `json:"data"`
	Pagination Pagination                `json:"pagination"`
	Facets     map[string]map[string]int `json:"facets,omitempty"`
}

// Config defines entity-specific query behavior.
type Config struct {
	SearchFields      []string
	AllowedSortFields []string
	AllowedFilters    []string
	FieldAliases      map[string]string
	DefaultSort       string
	FacetFields       []string
	FacetLabels       map[string]string
	IncludeConfig     IncludeConfig
}

// ResolveField returns the actual column name for a field, using FieldAliases if available.
func (c Config) ResolveField(field string) string {
	if c.FieldAliases != nil {
		if alias, ok := c.FieldAliases[field]; ok {
			return alias
		}
	}
	return field
}

// ResolveFacetLabel returns the display label for a facet field.
func (c Config) ResolveFacetLabel(field string) string {
	if c.FacetLabels != nil {
		if label, ok := c.FacetLabels[field]; ok {
			return label
		}
	}
	return field
}

// --- Include types ---

// IncludePath represents a parsed include path like "service.protocols".
type IncludePath struct {
	Parts []string
	Raw   string
}

// Root returns the first segment.
func (p IncludePath) Root() string {
	if len(p.Parts) == 0 {
		return ""
	}
	return p.Parts[0]
}

// HasChildren returns true if the path has nested segments.
func (p IncludePath) HasChildren() bool { return len(p.Parts) > 1 }

// Child returns a new IncludePath with the first segment removed.
func (p IncludePath) Child() IncludePath {
	if len(p.Parts) <= 1 {
		return IncludePath{}
	}
	childParts := p.Parts[1:]
	return IncludePath{Parts: childParts, Raw: strings.Join(childParts, ".")}
}

// Depth returns the nesting level.
func (p IncludePath) Depth() int { return len(p.Parts) }

// IncludeSet holds parsed include paths grouped by root.
type IncludeSet struct {
	Paths  []IncludePath
	ByRoot map[string][]IncludePath
}

// Has returns true if the given path (or any parent path) is included.
func (s IncludeSet) Has(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return false
	}
	rootPaths, ok := s.ByRoot[parts[0]]
	if !ok {
		return false
	}
	for _, p := range rootPaths {
		if p.Raw == path || strings.HasPrefix(p.Raw, path+".") {
			return true
		}
	}
	return false
}

// HasExact returns true only if the exact path is included.
func (s IncludeSet) HasExact(path string) bool {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return false
	}
	rootPaths, ok := s.ByRoot[parts[0]]
	if !ok {
		return false
	}
	for _, p := range rootPaths {
		if p.Raw == path {
			return true
		}
	}
	return false
}

// ChildrenOf returns include paths that are direct children of the given root.
func (s IncludeSet) ChildrenOf(root string) []IncludePath {
	rootPaths, ok := s.ByRoot[root]
	if !ok {
		return nil
	}
	var children []IncludePath
	for _, p := range rootPaths {
		if p.HasChildren() {
			children = append(children, p.Child())
		}
	}
	return children
}

// IsEmpty returns true if no includes were requested.
func (s IncludeSet) IsEmpty() bool { return len(s.Paths) == 0 }

// RelationType determines how a relationship should be loaded.
type RelationType string

const (
	RelationBelongsTo RelationType = "belongs_to"
	RelationHasMany   RelationType = "has_many"
	RelationHasOne    RelationType = "has_one"
)

// IncludeSpec defines how an include path maps to GORM loading operations.
type IncludeSpec struct {
	Type     RelationType
	Relation string
	Nested   map[string]IncludeSpec
	OrderBy  string
}

// IncludeConfig defines allowed includes for a resource.
type IncludeConfig struct {
	AllowedPaths []string
	MaxDepth     int
	Specs        map[string]IncludeSpec
}

// DefaultIncludeConfig returns a config with no includes allowed.
func DefaultIncludeConfig() IncludeConfig {
	return IncludeConfig{AllowedPaths: []string{}, MaxDepth: 3}
}

// IsPathAllowed checks if a path is allowed by the config.
func (c IncludeConfig) IsPathAllowed(path string) bool {
	if len(c.AllowedPaths) == 0 {
		return false
	}
	parts := strings.Split(path, ".")
	if c.MaxDepth > 0 && len(parts) > c.MaxDepth {
		return false
	}
	for _, allowed := range c.AllowedPaths {
		if matchPath(path, allowed) {
			return true
		}
	}
	return false
}

func matchPath(path, pattern string) bool {
	if path == pattern || pattern == "**" {
		return true
	}
	return matchParts(strings.Split(path, "."), strings.Split(pattern, "."))
}

func matchParts(pathParts, patternParts []string) bool {
	pi, pati := 0, 0
	for pi < len(pathParts) && pati < len(patternParts) {
		switch patternParts[pati] {
		case "**":
			if pati == len(patternParts)-1 {
				return true
			}
			for i := pi; i <= len(pathParts); i++ {
				if matchParts(pathParts[i:], patternParts[pati+1:]) {
					return true
				}
			}
			return false
		case "*":
			pi++
			pati++
		default:
			if pathParts[pi] != patternParts[pati] {
				return false
			}
			pi++
			pati++
		}
	}
	if pati < len(patternParts) && patternParts[pati] == "**" {
		return true
	}
	return pi == len(pathParts) && pati == len(patternParts)
}

// ParseIncludes parses an include string without HTTP request context.
func ParseIncludes(includeStr string, config IncludeConfig) IncludeSet {
	result := IncludeSet{Paths: []IncludePath{}, ByRoot: make(map[string][]IncludePath)}
	if includeStr == "" {
		return result
	}
	for _, p := range strings.Split(includeStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" || !config.IsPathAllowed(p) {
			continue
		}
		parts := strings.Split(p, ".")
		ip := IncludePath{Parts: parts, Raw: p}
		result.Paths = append(result.Paths, ip)
		result.ByRoot[ip.Root()] = append(result.ByRoot[ip.Root()], ip)
	}
	return result
}

// ParseIncludesFromRequest extracts _include from an HTTP request.
func ParseIncludesFromRequest(r *http.Request, config IncludeConfig) IncludeSet {
	return ParseIncludes(r.URL.Query().Get("_include"), config)
}
