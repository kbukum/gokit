package sqlite_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	. "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
)

// widget is the test model exercised by the GORM query builder.
type widget struct {
	ID       uint `gorm:"primaryKey"`
	Name     string
	Category string
	Price    int
	OwnerID  uint
	Owner    owner
}

type owner struct {
	ID      uint `gorm:"primaryKey"`
	Name    string
	Widgets []widget `gorm:"foreignKey:OwnerID"`
}

func newQueryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := Config{Enabled: true, DSN: ":memory:"}
	cfg.ApplyDefaults()
	log := logging.NewDefault("test")
	wrapped, err := NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { wrapped.Close() })
	db := wrapped.GormDB
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	// A :memory: database lives on a single connection; pin the pool so the
	// schema survives across parallel subtests sharing this handle.
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&owner{}, &widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	owners := []owner{{ID: 1, Name: "alice"}, {ID: 2, Name: "bob"}}
	if err := db.Create(&owners).Error; err != nil {
		t.Fatalf("seed owners: %v", err)
	}
	widgets := []widget{
		{Name: "alpha", Category: "tools", Price: 10, OwnerID: 1},
		{Name: "beta", Category: "tools", Price: 20, OwnerID: 1},
		{Name: "gamma", Category: "toys", Price: 30, OwnerID: 2},
		{Name: "delta", Category: "toys", Price: 40, OwnerID: 2},
	}
	if err := db.Create(&widgets).Error; err != nil {
		t.Fatalf("seed widgets: %v", err)
	}
	return db
}

func fullConfig() query.Config {
	return query.Config{
		SearchFields:      []string{"name"},
		AllowedSortFields: []string{"price", "name"},
		DefaultSort:       "id",
		FacetFields:       []string{"category"},
		FacetLabels:       map[string]string{"category": "Category"},
	}
}

func TestApplyToGorm_PaginatesAndCounts(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	params := query.Params{Page: 1, PageSize: 2}
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), params, fullConfig())
	if err != nil {
		t.Fatalf("ApplyToGorm: %v", err)
	}
	if res.Pagination.Total != 4 || res.Pagination.TotalPages != 2 {
		t.Fatalf("pagination = %+v", res.Pagination)
	}
	if len(res.Data) != 2 {
		t.Fatalf("len(data) = %d", len(res.Data))
	}
	if res.Facets["Category"]["tools"] != 2 || res.Facets["Category"]["toys"] != 2 {
		t.Fatalf("facets = %+v", res.Facets)
	}
}

func TestApplyToGorm_NoPagination(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{NoPagination: true}, fullConfig())
	if err != nil {
		t.Fatalf("ApplyToGorm: %v", err)
	}
	if len(res.Data) != 4 || res.Pagination.TotalPages != 1 || res.Pagination.PageSize != 4 {
		t.Fatalf("no-pagination = %+v / len=%d", res.Pagination, len(res.Data))
	}
}

func TestApplyToGorm_FreeTextSearch(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	params := query.Params{Page: 1, PageSize: 10, Query: query.FilterQuery{FreeText: "ALP"}}
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), params, fullConfig())
	if err != nil {
		t.Fatalf("ApplyToGorm: %v", err)
	}
	if len(res.Data) != 1 || res.Data[0].Name != "alpha" {
		t.Fatalf("search result = %+v", res.Data)
	}
}

func TestApplyToGorm_SortAscDesc(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	cfg := fullConfig()
	desc, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{Page: 1, PageSize: 10, SortBy: "price", SortOrder: "desc"}, cfg)
	if err != nil {
		t.Fatalf("desc: %v", err)
	}
	if desc.Data[0].Price != 40 {
		t.Fatalf("desc first price = %d", desc.Data[0].Price)
	}
	asc, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{Page: 1, PageSize: 10, SortBy: "price", SortOrder: "asc"}, cfg)
	if err != nil {
		t.Fatalf("asc: %v", err)
	}
	if asc.Data[0].Price != 10 {
		t.Fatalf("asc first price = %d", asc.Data[0].Price)
	}
}

func TestApplyToGorm_DefaultSortAndNoMatch(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	cfg := query.Config{DefaultSort: "price DESC"}
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{Page: 1, PageSize: 10, SortBy: "unknown"}, cfg)
	if err != nil {
		t.Fatalf("default sort: %v", err)
	}
	if res.Data[0].Price != 40 {
		t.Fatalf("default sort not applied: %d", res.Data[0].Price)
	}
	untouched, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{Page: 1, PageSize: 10}, query.Config{})
	if err != nil {
		t.Fatalf("untouched sort: %v", err)
	}
	if len(untouched.Data) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(untouched.Data))
	}
}

func TestApplyToGorm_UnsafeSortAliasFailsClosed(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	cfg := query.Config{
		AllowedSortFields: []string{"evil"},
		FieldAliases:      map[string]string{"evil": "price; DROP TABLE widgets;--"},
	}
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), query.Params{Page: 1, PageSize: 10, SortBy: "evil", SortOrder: "asc"}, cfg)
	if err != nil {
		t.Fatalf("unsafe sort: %v", err)
	}
	if len(res.Data) != 4 {
		t.Fatalf("expected fail-closed to return all rows, got %d", len(res.Data))
	}
}

func TestApplyCondition_AllOperators(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	cases := []struct {
		name string
		cond query.Condition
		want int
	}{
		{"eq", query.Condition{Field: "category", Operator: query.OpEq, Value: "tools"}, 2},
		{"eq-in-values", query.Condition{Field: "category", Operator: query.OpEq, Values: []string{"tools", "toys"}}, 4},
		{"neq", query.Condition{Field: "category", Operator: query.OpNeq, Value: "tools"}, 2},
		{"neq-values", query.Condition{Field: "category", Operator: query.OpNeq, Values: []string{"tools"}}, 2},
		{"gt", query.Condition{Field: "price", Operator: query.OpGt, Value: "20"}, 2},
		{"gte", query.Condition{Field: "price", Operator: query.OpGte, Value: "20"}, 3},
		{"lt", query.Condition{Field: "price", Operator: query.OpLt, Value: "20"}, 1},
		{"lte", query.Condition{Field: "price", Operator: query.OpLte, Value: "20"}, 2},
		{"in-values", query.Condition{Field: "price", Operator: query.OpIn, Values: []string{"10", "40"}}, 2},
		{"in-csv", query.Condition{Field: "price", Operator: query.OpIn, Value: "10,40"}, 2},
		{"nin-values", query.Condition{Field: "price", Operator: query.OpNin, Values: []string{"10", "40"}}, 2},
		{"nin-csv", query.Condition{Field: "price", Operator: query.OpNin, Value: "10,40"}, 2},
		{"like", query.Condition{Field: "name", Operator: query.OpLike, Value: "alph"}, 1},
		{"ilike", query.Condition{Field: "name", Operator: query.OpIlike, Value: "ALPH"}, 1},
		{"notnull", query.Condition{Field: "name", Operator: query.OpNotNull}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out []widget
			q := query.ApplyConditions(db.Model(&widget{}), []query.Condition{tc.cond}, query.Config{})
			if err := q.Find(&out).Error; err != nil {
				t.Fatalf("find: %v", err)
			}
			if len(out) != tc.want {
				t.Fatalf("%s: got %d, want %d", tc.name, len(out), tc.want)
			}
		})
	}
}

func TestApplyCondition_NullOperator(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	var out []widget
	q := query.ApplyConditions(db.Model(&widget{}), []query.Condition{{Field: "name", Operator: query.OpNull}}, query.Config{})
	if err := q.Find(&out).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 null names, got %d", len(out))
	}
}

func TestApplyCondition_UnsafeFieldFailsClosed(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	malicious := query.Condition{Field: "price); DROP TABLE widgets;--", Operator: query.OpEq, Value: "10"}
	var out []widget
	q := query.ApplyConditions(db.Model(&widget{}), []query.Condition{malicious}, query.Config{})
	if err := q.Find(&out).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected fail-closed to return all rows, got %d", len(out))
	}
	var count int64
	if err := db.Model(&widget{}).Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("table should be intact: count=%d err=%v", count, err)
	}
}

func TestApplySearch_UnsafeFieldSkipped(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	params := query.Params{Page: 1, PageSize: 10, Query: query.FilterQuery{FreeText: "x"}}
	cfg := query.Config{SearchFields: []string{"name); DROP TABLE widgets;--"}}
	res, err := query.ApplyToGorm[widget](db.Model(&widget{}), params, cfg)
	if err != nil {
		t.Fatalf("ApplyToGorm: %v", err)
	}
	if len(res.Data) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(res.Data))
	}
}

func TestComputeFacetsWithFilters_CrossFilter(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	conds := []query.Condition{{Field: "category", Operator: query.OpEq, Value: "tools"}}
	cfg := query.Config{FacetFields: []string{"category"}, FacetLabels: map[string]string{"category": "Category"}}
	facets := query.ComputeFacetsWithFilters(db.Model(&widget{}), cfg.FacetFields, conds, cfg)
	if facets["Category"]["tools"] != 2 || facets["Category"]["toys"] != 2 {
		t.Fatalf("cross-filter facets = %+v", facets)
	}
}

func TestComputeFacetsWithFilters_NilAndUnsafe(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	if query.ComputeFacetsWithFilters(db.Model(&widget{}), nil, nil, query.Config{}) != nil {
		t.Fatal("expected nil for no facet fields")
	}
	f := query.ComputeFacetsWithFilters(db.Model(&widget{}), []string{"cat; DROP--"}, nil, query.Config{})
	if len(f) != 0 {
		t.Fatalf("unsafe facet field should be skipped, got %+v", f)
	}
}

func TestApplyIncludes(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	widgetCfg := query.IncludeConfig{
		Specs: map[string]query.IncludeSpec{
			"owner": {Type: query.RelationBelongsTo, Relation: "Owner"},
			"empty": {Type: query.RelationBelongsTo, Relation: ""},
		},
	}
	widgetIncludes := query.IncludeSet{Paths: []query.IncludePath{
		{Raw: "owner"}, {Raw: "empty"}, {Raw: "unknown"},
	}}
	var widgets []widget
	if err := query.ApplyIncludes(db.Model(&widget{}), widgetIncludes, widgetCfg).Find(&widgets).Error; err != nil {
		t.Fatalf("find widgets with includes: %v", err)
	}
	if len(widgets) != 4 {
		t.Fatalf("expected 4 widgets, got %d", len(widgets))
	}

	// has_many with a nested belongs_to exercises the preload builder's nested path.
	ownerCfg := query.IncludeConfig{
		Specs: map[string]query.IncludeSpec{
			"widgets": {
				Type:     query.RelationHasMany,
				Relation: "Widgets",
				OrderBy:  "price",
				Nested:   map[string]query.IncludeSpec{"owner": {Type: query.RelationBelongsTo, Relation: "Owner"}},
			},
		},
	}
	ownerIncludes := query.IncludeSet{Paths: []query.IncludePath{{Raw: "widgets"}}}
	var owners []owner
	if err := query.ApplyIncludes(db.Model(&owner{}), ownerIncludes, ownerCfg).Find(&owners).Error; err != nil {
		t.Fatalf("find owners with includes: %v", err)
	}
	if len(owners) != 2 || len(owners[0].Widgets) != 2 {
		t.Fatalf("expected 2 owners each with 2 widgets, got %+v", owners)
	}
}

func TestApplyIncludes_EmptyReturnsUnchanged(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	base := db.Model(&widget{})
	if query.ApplyIncludes(base, query.IncludeSet{}, query.IncludeConfig{}) != base {
		t.Fatal("empty includes should return db unchanged")
	}
	cfg := query.IncludeConfig{Specs: map[string]query.IncludeSpec{"x": {Relation: "X"}}}
	if query.ApplyIncludes(base, query.IncludeSet{}, cfg) != base {
		t.Fatal("empty include set should return db unchanged")
	}
}

func TestApplyIncludesFromParams(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	cfg := query.Config{IncludeConfig: query.IncludeConfig{
		Specs: map[string]query.IncludeSpec{"owner": {Type: query.RelationBelongsTo, Relation: "Owner"}},
	}}
	params := query.Params{Includes: query.IncludeSet{Paths: []query.IncludePath{{Raw: "owner"}}}}
	q := query.ApplyIncludesFromParams(db.Model(&widget{}), params, cfg)
	var out []widget
	if err := q.Find(&out).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(out))
	}
}

func TestApplyIncludes_HasOneAndUnknownType(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	hasOneCfg := query.IncludeConfig{
		Specs: map[string]query.IncludeSpec{"owner": {Type: query.RelationHasOne, Relation: "Owner"}},
	}
	includes := query.IncludeSet{Paths: []query.IncludePath{{Raw: "owner"}}}
	var out []widget
	if err := query.ApplyIncludes(db.Model(&widget{}), includes, hasOneCfg).Find(&out).Error; err != nil {
		t.Fatalf("has_one include: %v", err)
	}

	// An unknown relation type falls through to a plain join.
	unknownCfg := query.IncludeConfig{
		Specs: map[string]query.IncludeSpec{"owner": {Type: query.RelationType("weird"), Relation: "Owner"}},
	}
	if err := query.ApplyIncludes(db.Model(&widget{}), includes, unknownCfg).Find(&out).Error; err != nil {
		t.Fatalf("unknown-type include: %v", err)
	}
}

func TestApplyToGorm_CountError(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	type missing struct{ ID uint }
	_, err := query.ApplyToGorm[missing](db.Model(&missing{}), query.Params{Page: 1, PageSize: 10}, query.Config{})
	if err == nil {
		t.Fatal("expected count error on missing table")
	}
}

func TestApplyToGorm_FindError(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	type mismatch struct {
		ID  uint
		Bad complex128
	}
	_, err := query.ApplyToGorm[mismatch](db.Model(&mismatch{}), query.Params{Page: 1, PageSize: 10}, query.Config{})
	if err == nil {
		t.Fatal("expected error for unmigrated destination")
	}
}
