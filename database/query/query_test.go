package query

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// --- Operator ---

func TestOperator_IsValid(t *testing.T) {
	valid := []Operator{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIn, OpNin, OpLike, OpIlike, OpNull, OpNotNull}
	for _, op := range valid {
		if !op.IsValid() {
			t.Errorf("expected %q to be valid", op)
		}
	}
}

func TestOperator_IsValid_Invalid(t *testing.T) {
	invalid := []Operator{"unknown", "between", ""}
	for _, op := range invalid {
		if op.IsValid() {
			t.Errorf("expected %q to be invalid", op)
		}
	}
}

func TestAllOperators_Count(t *testing.T) {
	ops := AllOperators()
	if len(ops) != 12 {
		t.Errorf("AllOperators() returned %d, want 12", len(ops))
	}
}

// --- Config ---

func TestConfig_ResolveField_WithAlias(t *testing.T) {
	cfg := Config{FieldAliases: map[string]string{"status": "current_status"}}
	if got := cfg.ResolveField("status"); got != "current_status" {
		t.Errorf("ResolveField(status) = %q, want %q", got, "current_status")
	}
}

func TestConfig_ResolveField_WithoutAlias(t *testing.T) {
	cfg := Config{FieldAliases: map[string]string{"status": "current_status"}}
	if got := cfg.ResolveField("name"); got != "name" {
		t.Errorf("ResolveField(name) = %q, want %q", got, "name")
	}
}

func TestConfig_ResolveField_NilAliases(t *testing.T) {
	cfg := Config{}
	if got := cfg.ResolveField("name"); got != "name" {
		t.Errorf("ResolveField(name) = %q, want %q", got, "name")
	}
}

func TestConfig_ResolveFacetLabel_WithLabel(t *testing.T) {
	cfg := Config{FacetLabels: map[string]string{"status": "Status Filter"}}
	if got := cfg.ResolveFacetLabel("status"); got != "Status Filter" {
		t.Errorf("got %q, want %q", got, "Status Filter")
	}
}

func TestConfig_ResolveFacetLabel_WithoutLabel(t *testing.T) {
	cfg := Config{FacetLabels: map[string]string{}}
	if got := cfg.ResolveFacetLabel("status"); got != "status" {
		t.Errorf("got %q, want %q", got, "status")
	}
}

func TestConfig_ResolveFacetLabel_NilLabels(t *testing.T) {
	cfg := Config{}
	if got := cfg.ResolveFacetLabel("field"); got != "field" {
		t.Errorf("got %q, want %q", got, "field")
	}
}

// --- IncludePath ---

func TestIncludePath_Root(t *testing.T) {
	p := IncludePath{Parts: []string{"service", "protocols"}, Raw: "service.protocols"}
	if got := p.Root(); got != "service" {
		t.Errorf("Root() = %q, want %q", got, "service")
	}
}

func TestIncludePath_Root_Empty(t *testing.T) {
	p := IncludePath{}
	if got := p.Root(); got != "" {
		t.Errorf("Root() = %q, want empty", got)
	}
}

func TestIncludePath_HasChildren(t *testing.T) {
	multi := IncludePath{Parts: []string{"a", "b"}}
	if !multi.HasChildren() {
		t.Error("expected HasChildren() = true for multi-part path")
	}
	single := IncludePath{Parts: []string{"a"}}
	if single.HasChildren() {
		t.Error("expected HasChildren() = false for single-part path")
	}
}

func TestIncludePath_Child(t *testing.T) {
	p := IncludePath{Parts: []string{"service", "protocols", "details"}, Raw: "service.protocols.details"}
	child := p.Child()
	if child.Raw != "protocols.details" {
		t.Errorf("Child().Raw = %q, want %q", child.Raw, "protocols.details")
	}
	if len(child.Parts) != 2 || child.Parts[0] != "protocols" {
		t.Errorf("Child().Parts = %v, want [protocols details]", child.Parts)
	}
}

func TestIncludePath_Child_SinglePart(t *testing.T) {
	p := IncludePath{Parts: []string{"service"}, Raw: "service"}
	child := p.Child()
	if child.Raw != "" {
		t.Errorf("Child().Raw = %q, want empty", child.Raw)
	}
	if len(child.Parts) != 0 {
		t.Errorf("Child().Parts should be empty, got %v", child.Parts)
	}
}

func TestIncludePath_Depth(t *testing.T) {
	tests := []struct {
		parts []string
		want  int
	}{
		{nil, 0},
		{[]string{"a"}, 1},
		{[]string{"a", "b", "c"}, 3},
	}
	for _, tt := range tests {
		p := IncludePath{Parts: tt.parts}
		if got := p.Depth(); got != tt.want {
			t.Errorf("Depth() with %v = %d, want %d", tt.parts, got, tt.want)
		}
	}
}

// --- IncludeSet ---

func TestIncludeSet_IsEmpty(t *testing.T) {
	empty := IncludeSet{Paths: []IncludePath{}}
	if !empty.IsEmpty() {
		t.Error("expected IsEmpty() = true")
	}
	nonEmpty := IncludeSet{Paths: []IncludePath{{Parts: []string{"a"}, Raw: "a"}}}
	if nonEmpty.IsEmpty() {
		t.Error("expected IsEmpty() = false")
	}
}

func TestIncludeSet_Has(t *testing.T) {
	set := IncludeSet{
		Paths: []IncludePath{
			{Parts: []string{"service", "protocols"}, Raw: "service.protocols"},
		},
		ByRoot: map[string][]IncludePath{
			"service": {{Parts: []string{"service", "protocols"}, Raw: "service.protocols"}},
		},
	}
	if !set.Has("service") {
		t.Error("Has(service) should be true (parent of service.protocols)")
	}
	if !set.Has("service.protocols") {
		t.Error("Has(service.protocols) should be true (exact match)")
	}
	if set.Has("other") {
		t.Error("Has(other) should be false")
	}
}

func TestIncludeSet_HasExact(t *testing.T) {
	set := IncludeSet{
		Paths: []IncludePath{
			{Parts: []string{"service", "protocols"}, Raw: "service.protocols"},
		},
		ByRoot: map[string][]IncludePath{
			"service": {{Parts: []string{"service", "protocols"}, Raw: "service.protocols"}},
		},
	}
	if !set.HasExact("service.protocols") {
		t.Error("HasExact(service.protocols) should be true")
	}
	if set.HasExact("service") {
		t.Error("HasExact(service) should be false (only child exists)")
	}
}

func TestIncludeSet_ChildrenOf(t *testing.T) {
	set := IncludeSet{
		Paths: []IncludePath{
			{Parts: []string{"service", "protocols"}, Raw: "service.protocols"},
			{Parts: []string{"service"}, Raw: "service"},
		},
		ByRoot: map[string][]IncludePath{
			"service": {
				{Parts: []string{"service", "protocols"}, Raw: "service.protocols"},
				{Parts: []string{"service"}, Raw: "service"},
			},
		},
	}
	children := set.ChildrenOf("service")
	if len(children) != 1 {
		t.Fatalf("ChildrenOf(service) returned %d, want 1", len(children))
	}
	if children[0].Raw != "protocols" {
		t.Errorf("child.Raw = %q, want %q", children[0].Raw, "protocols")
	}
}

func TestIncludeSet_ChildrenOf_NoChildren(t *testing.T) {
	set := IncludeSet{
		Paths:  []IncludePath{{Parts: []string{"service"}, Raw: "service"}},
		ByRoot: map[string][]IncludePath{"service": {{Parts: []string{"service"}, Raw: "service"}}},
	}
	children := set.ChildrenOf("service")
	if len(children) != 0 {
		t.Errorf("expected no children, got %d", len(children))
	}
}

func TestIncludeSet_ChildrenOf_UnknownRoot(t *testing.T) {
	set := IncludeSet{ByRoot: map[string][]IncludePath{}}
	if children := set.ChildrenOf("unknown"); children != nil {
		t.Errorf("expected nil, got %v", children)
	}
}

// --- IncludeConfig.IsPathAllowed ---

func TestIncludeConfig_IsPathAllowed_ExactMatch(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service", "service.protocols"}, MaxDepth: 3}
	if !cfg.IsPathAllowed("service") {
		t.Error("exact match should be allowed")
	}
	if !cfg.IsPathAllowed("service.protocols") {
		t.Error("exact nested match should be allowed")
	}
}

func TestIncludeConfig_IsPathAllowed_Wildcard(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service.*"}, MaxDepth: 3}
	if !cfg.IsPathAllowed("service.protocols") {
		t.Error("wildcard * should match single segment")
	}
	if cfg.IsPathAllowed("service.protocols.details") {
		t.Error("wildcard * should not match multiple segments")
	}
}

func TestIncludeConfig_IsPathAllowed_GlobStar(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service.**"}, MaxDepth: 5}
	if !cfg.IsPathAllowed("service.protocols") {
		t.Error("** should match single nested segment")
	}
	if !cfg.IsPathAllowed("service.protocols.details") {
		t.Error("** should match multiple nested segments")
	}
}

func TestIncludeConfig_IsPathAllowed_MaxDepthExceeded(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"**"}, MaxDepth: 2}
	if !cfg.IsPathAllowed("a.b") {
		t.Error("depth 2 should be allowed with MaxDepth=2")
	}
	if cfg.IsPathAllowed("a.b.c") {
		t.Error("depth 3 should exceed MaxDepth=2")
	}
}

func TestIncludeConfig_IsPathAllowed_EmptyAllowed(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{}, MaxDepth: 3}
	if cfg.IsPathAllowed("anything") {
		t.Error("empty AllowedPaths should reject all")
	}
}

func TestIncludeConfig_IsPathAllowed_DoubleStarOnly(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"**"}, MaxDepth: 10}
	if !cfg.IsPathAllowed("any.path.at.all") {
		t.Error("** alone should match everything within MaxDepth")
	}
}

// --- ParseIncludes ---

func TestParseIncludes_Empty(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"**"}, MaxDepth: 3}
	set := ParseIncludes("", cfg)
	if !set.IsEmpty() {
		t.Error("empty string should produce empty set")
	}
}

func TestParseIncludes_SinglePath(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service"}, MaxDepth: 3}
	set := ParseIncludes("service", cfg)
	if len(set.Paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(set.Paths))
	}
	if set.Paths[0].Raw != "service" {
		t.Errorf("path = %q, want %q", set.Paths[0].Raw, "service")
	}
}

func TestParseIncludes_MultiplePaths(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service", "tags"}, MaxDepth: 3}
	set := ParseIncludes("service,tags", cfg)
	if len(set.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(set.Paths))
	}
}

func TestParseIncludes_DisallowedFiltered(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service"}, MaxDepth: 3}
	set := ParseIncludes("service,forbidden", cfg)
	if len(set.Paths) != 1 {
		t.Fatalf("expected 1 path (forbidden filtered out), got %d", len(set.Paths))
	}
	if set.Paths[0].Raw != "service" {
		t.Errorf("path = %q, want %q", set.Paths[0].Raw, "service")
	}
}

func TestParseIncludes_WhitespaceHandling(t *testing.T) {
	cfg := IncludeConfig{AllowedPaths: []string{"service", "tags"}, MaxDepth: 3}
	set := ParseIncludes("service , tags", cfg)
	if len(set.Paths) != 2 {
		t.Fatalf("expected 2 paths with whitespace trimming, got %d", len(set.Paths))
	}
}

// --- ParseFromRequest / parseCondition ---

func makeRequest(queryString string) *http.Request {
	r := httptest.NewRequest("GET", "/?"+queryString, http.NoBody)
	return r
}

func TestParseFromRequest_Defaults(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest(""), cfg)
	if params.Page != 1 {
		t.Errorf("Page = %d, want 1", params.Page)
	}
	if params.PageSize != DefaultPageSize {
		t.Errorf("PageSize = %d, want %d", params.PageSize, DefaultPageSize)
	}
	if params.SortOrder != "asc" {
		t.Errorf("SortOrder = %q, want %q", params.SortOrder, "asc")
	}
}

func TestParseFromRequest_IsNull(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"deleted_at"}}
	params := ParseFromRequest(makeRequest("deleted_at=is.null"), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	c := params.Query.Conditions[0]
	if c.Operator != OpNull {
		t.Errorf("operator = %q, want %q", c.Operator, OpNull)
	}
	if c.Field != "deleted_at" {
		t.Errorf("field = %q, want %q", c.Field, "deleted_at")
	}
}

func TestParseFromRequest_NotIsNull(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"deleted_at"}}
	params := ParseFromRequest(makeRequest("deleted_at=not.is.null"), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	if params.Query.Conditions[0].Operator != OpNotNull {
		t.Errorf("operator = %q, want %q", params.Query.Conditions[0].Operator, OpNotNull)
	}
}

func TestParseFromRequest_EqValue(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"status"}}
	params := ParseFromRequest(makeRequest("status=eq.active"), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	c := params.Query.Conditions[0]
	if c.Operator != OpEq || c.Value != "active" {
		t.Errorf("got op=%q val=%q, want op=eq val=active", c.Operator, c.Value)
	}
}

func TestParseFromRequest_InArray(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"status"}}
	params := ParseFromRequest(makeRequest("status="+url.QueryEscape("in.(a,b,c)")), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	c := params.Query.Conditions[0]
	if c.Operator != OpIn {
		t.Errorf("operator = %q, want %q", c.Operator, OpIn)
	}
	if len(c.Values) != 3 {
		t.Errorf("values count = %d, want 3", len(c.Values))
	}
}

func TestParseFromRequest_UnknownOpDefaultsToEq(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"field"}}
	params := ParseFromRequest(makeRequest("field=unknown.val"), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	c := params.Query.Conditions[0]
	if c.Operator != OpEq {
		t.Errorf("operator = %q, want eq for unknown op", c.Operator)
	}
	if c.Value != "unknown.val" {
		t.Errorf("value = %q, want %q", c.Value, "unknown.val")
	}
}

func TestParseFromRequest_PlainValue(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"name"}}
	params := ParseFromRequest(makeRequest("name=alice"), cfg)
	if len(params.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(params.Query.Conditions))
	}
	c := params.Query.Conditions[0]
	if c.Operator != OpEq || c.Value != "alice" {
		t.Errorf("got op=%q val=%q, want op=eq val=alice", c.Operator, c.Value)
	}
}

// --- Sort order normalization ---

func TestParseFromRequest_SortOrderDesc(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("order=desc"), cfg)
	if params.SortOrder != "desc" {
		t.Errorf("SortOrder = %q, want %q", params.SortOrder, "desc")
	}
}

func TestParseFromRequest_SortOrderDescCaseInsensitive(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("order=DESC"), cfg)
	if params.SortOrder != "desc" {
		t.Errorf("SortOrder = %q, want %q", params.SortOrder, "desc")
	}
}

func TestParseFromRequest_SortOrderDefaultsToAsc(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("order=invalid"), cfg)
	if params.SortOrder != "asc" {
		t.Errorf("SortOrder = %q, want %q", params.SortOrder, "asc")
	}
}

// --- Pagination ---

func TestParseFromRequest_PageSizeClamped(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("limit=999"), cfg)
	if params.PageSize != MaxPageSize {
		t.Errorf("PageSize = %d, want %d (clamped)", params.PageSize, MaxPageSize)
	}
}

func TestParseFromRequest_PageDefaultsTo1(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("page=0"), cfg)
	if params.Page != 1 {
		t.Errorf("Page = %d, want 1 (default for invalid)", params.Page)
	}
}

func TestParseFromRequest_NoPagination_LimitMinus1(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("limit=-1"), cfg)
	if !params.NoPagination {
		t.Error("NoPagination should be true for limit=-1")
	}
}

func TestParseFromRequest_NoPagination_LimitAll(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("limit=all"), cfg)
	if !params.NoPagination {
		t.Error("NoPagination should be true for limit=all")
	}
}

func TestParseFromRequest_NoPagination_PageSizeMinus1(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("pageSize=-1"), cfg)
	if !params.NoPagination {
		t.Error("NoPagination should be true for pageSize=-1")
	}
}

func TestParseFromRequest_NoPagination_PageSizeAll(t *testing.T) {
	cfg := Config{}
	params := ParseFromRequest(makeRequest("pageSize=all"), cfg)
	if !params.NoPagination {
		t.Error("NoPagination should be true for pageSize=all")
	}
}

// --- Search ---

func TestParseFromRequest_SearchFreeText(t *testing.T) {
	cfg := Config{SearchFields: []string{"name", "description"}}
	params := ParseFromRequest(makeRequest("search=hello+world"), cfg)
	if params.Query.FreeText != "hello world" {
		t.Errorf("FreeText = %q, want %q", params.Query.FreeText, "hello world")
	}
}

// --- Filter string ---

func TestParseFromRequest_FilterString(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"status", "priority"}}
	params := ParseFromRequest(makeRequest("filter="+url.QueryEscape("status=eq.active&priority=gt.3")), cfg)
	if len(params.Query.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(params.Query.Conditions))
	}
}

// --- AddCondition ---

func TestParams_AddCondition(t *testing.T) {
	p := &Params{Query: FilterQuery{Conditions: []Condition{}}}
	p.AddCondition("name", OpEq, "test")
	if len(p.Query.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(p.Query.Conditions))
	}
	c := p.Query.Conditions[0]
	if c.Field != "name" || c.Operator != OpEq || c.Value != "test" {
		t.Errorf("condition = %+v, want field=name op=eq value=test", c)
	}
}

// --- matchPath ---

func TestMatchPath_ExactMatch(t *testing.T) {
	if !matchPath("service", "service") {
		t.Error("exact match should return true")
	}
}

func TestMatchPath_DoubleStarAlone(t *testing.T) {
	if !matchPath("anything.here", "**") {
		t.Error("** alone should match any path")
	}
}

func TestMatchPath_NoMatch(t *testing.T) {
	if matchPath("service.protocols", "other.path") {
		t.Error("non-matching paths should return false")
	}
}

func TestMatchParts_TrailingDoubleStar(t *testing.T) {
	// Pattern "a.**" should match "a" (trailing ** can match zero parts)
	if !matchParts([]string{"a"}, []string{"a", "**"}) {
		t.Error("trailing ** should match zero additional parts")
	}
}

// --- ParseIncludesFromRequest ---

func TestParseIncludesFromRequest(t *testing.T) {
	cfg := Config{IncludeConfig: IncludeConfig{AllowedPaths: []string{"service"}, MaxDepth: 3}}
	r := makeRequest("_include=service")
	params := ParseFromRequest(r, cfg)
	if params.Includes.IsEmpty() {
		t.Error("expected includes to be parsed from request")
	}
	if !params.Includes.HasExact("service") {
		t.Error("expected service to be in includes")
	}
}

// --- Disallowed filter field ---

func TestParseFromRequest_DisallowedFilter(t *testing.T) {
	cfg := Config{AllowedFilters: []string{"status"}}
	params := ParseFromRequest(makeRequest("secret=eq.hack"), cfg)
	if len(params.Query.Conditions) != 0 {
		t.Error("disallowed filter field should be ignored")
	}
}

func TestParseCondition_EscapedValue(t *testing.T) {
	t.Parallel()
	// like.a\.b => value should unescape the backslash, keeping the dot.
	cond := parseCondition("name", `like.a\.b`)
	if cond == nil || cond.Operator != OpLike || cond.Value != "a.b" {
		t.Fatalf("cond = %+v", cond)
	}
}

func TestParseArrayValues_EscapesAndTrimming(t *testing.T) {
	t.Parallel()
	got := parseArrayValues(`a, b\,c , ,d`)
	want := []string{"a", "b,c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseArrayValues_TrailingEscape(t *testing.T) {
	t.Parallel()
	// A dangling backslash at the end must not panic and is dropped.
	got := parseArrayValues(`x\`)
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got %#v", got)
	}
}

func TestParseFilterString_MalformedAndDisallowed(t *testing.T) {
	t.Parallel()
	// No '=' => skipped; disallowed field => skipped; valid => kept.
	conds := parseFilterString("bogus&status=eq.active&secret=eq.x", []string{"status"})
	if len(conds) != 1 || conds[0].Field != "status" {
		t.Fatalf("conds = %+v", conds)
	}
}

func TestIsFieldAllowed(t *testing.T) {
	t.Parallel()
	if !isFieldAllowed("anything", nil) {
		t.Fatal("empty allow-list should permit all")
	}
	if isFieldAllowed("x", []string{"y"}) {
		t.Fatal("field not in allow-list should be rejected")
	}
	if !isFieldAllowed("y", []string{"y"}) {
		t.Fatal("field in allow-list should be permitted")
	}
}

func TestConfigPageSizeOverrides(t *testing.T) {
	t.Parallel()
	c := Config{DefaultPageSize: 7, MaxPageSize: 9}
	if c.defaultPageSize() != 7 {
		t.Fatalf("defaultPageSize = %d", c.defaultPageSize())
	}
	if c.maxPageSize() != 9 {
		t.Fatalf("maxPageSize = %d", c.maxPageSize())
	}
	empty := Config{}
	if empty.defaultPageSize() != DefaultPageSize || empty.maxPageSize() != MaxPageSize {
		t.Fatalf("empty defaults wrong: %d/%d", empty.defaultPageSize(), empty.maxPageSize())
	}
}

func TestClampUpperBound(t *testing.T) {
	t.Parallel()
	if clamp(500, 1, 100) != 100 {
		t.Fatal("expected upper clamp")
	}
	if clamp(-5, 1, 100) != 1 {
		t.Fatal("expected lower clamp")
	}
	if clamp(50, 1, 100) != 50 {
		t.Fatal("expected passthrough")
	}
}

func TestDefaultIncludeConfig(t *testing.T) {
	t.Parallel()
	c := DefaultIncludeConfig()
	if c.MaxDepth != 3 || len(c.AllowedPaths) != 0 {
		t.Fatalf("DefaultIncludeConfig = %+v", c)
	}
	if c.IsPathAllowed("anything") {
		t.Fatal("no paths allowed by default")
	}
}

func TestIncludeSet_HasAndHasExact_PrefixMatch(t *testing.T) {
	t.Parallel()
	set := IncludeSet{
		Paths:  []IncludePath{{Parts: []string{"a", "b"}, Raw: "a.b"}},
		ByRoot: map[string][]IncludePath{"a": {{Parts: []string{"a", "b"}, Raw: "a.b"}}},
	}
	if !set.Has("a") {
		t.Fatal("Has(parent) should match prefix")
	}
	if !set.Has("a.b") || !set.HasExact("a.b") {
		t.Fatal("exact path should match")
	}
	if set.HasExact("a") {
		t.Fatal("HasExact(parent) should not match")
	}
	if set.Has("z") || set.HasExact("z") {
		t.Fatal("unknown root should not match")
	}
}

func TestMatchParts_DoubleStarInMiddle(t *testing.T) {
	t.Parallel()
	// "**" consumes intermediate segments and matches the trailing tail.
	if !matchPath("a.b.c.d", "a.**.d") {
		t.Fatal("expected ** to bridge middle segments")
	}
	if matchPath("a.b.c", "a.x.c") {
		t.Fatal("literal mismatch should fail")
	}
	if matchPath("a.b", "a.b.c") {
		t.Fatal("pattern longer than path should fail")
	}
}

func TestParseFromRequest_ConfiguredPageSizeParam(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?page_size=5", http.NoBody)
	p := ParseFromRequest(r, Config{})
	if p.PageSize != 5 {
		t.Fatalf("PageSize = %d", p.PageSize)
	}
}
