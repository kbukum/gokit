package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/tool"
)

// --- Test types ---

type SearchInput struct {
	Query    string `json:"query"    jsonschema:"required,description=Search text"`
	Platform string `json:"platform" jsonschema:"enum=youtube,enum=tiktok"`
	Limit    int    `json:"limit"    jsonschema:"minimum=1,maximum=100"`
}

type SearchOutput struct {
	Items []string `json:"items"`
	Total int      `json:"total"`
}

func doSearch(ctx context.Context, in SearchInput) (SearchOutput, error) {
	return SearchOutput{
		Items: []string{"result1", "result2"},
		Total: 2,
	}, nil
}

// --- FromFunc tests ---

func TestFromFunc(t *testing.T) {
	st := tool.FromFunc("search", "Search for content", doSearch)

	def := st.Definition()
	if def.Name != "search" {
		t.Errorf("expected name 'search', got %q", def.Name)
	}
	if def.Description != "Search for content" {
		t.Errorf("expected description, got %q", def.Description)
	}
	if def.InputSchema == nil {
		t.Error("expected non-nil input schema")
	}
	if def.OutputSchema == nil {
		t.Error("expected non-nil output schema")
	}
	if def.InputSchema["type"] != "object" {
		t.Errorf("expected input type=object, got %v", def.InputSchema["type"])
	}
}

func TestFromFunc_Execute(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch)

	out, err := st.Execute(context.Background(), SearchInput{
		Query: "test", Platform: "youtube", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 {
		t.Errorf("expected total=2, got %d", out.Total)
	}
	if len(out.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(out.Items))
	}
}

func TestFromFunc_WithAnnotations(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch).
		WithAnnotations(tool.Annotations{
			Category: "discovery",
			Tags:     []string{"search", "content"},
		})

	def := st.Definition()
	if def.Annotations.Category != "discovery" {
		t.Errorf("expected category, got %q", def.Annotations.Category)
	}
}

// --- Callable tests ---

func TestCallable(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch)
	c := st.AsCallable()

	if c.Definition().Name != "search" {
		t.Errorf("expected name 'search', got %q", c.Definition().Name)
	}

	input := `{"query":"hello","platform":"youtube","limit":5}`
	result, err := c.Call(tool.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatal(err)
	}

	var out SearchOutput
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 {
		t.Errorf("expected total=2, got %d", out.Total)
	}
}

func TestCallable_InvalidJSON(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch)
	c := st.AsCallable()

	_, err := c.Call(tool.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCallable_Validate(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch)
	c := st.AsCallable()

	// Valid input
	vr := c.Validate(json.RawMessage(`{"query":"hello"}`))
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}

	// Invalid JSON
	vr = c.Validate(json.RawMessage(`{invalid`))
	if vr.Valid {
		t.Error("expected invalid for malformed JSON")
	}
}

// --- Context tests ---

func TestContext_Background(t *testing.T) {
	ctx := tool.Background()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Err() != nil {
		t.Error("background context should not be canceled")
	}
}

func TestContext_NewContext(t *testing.T) {
	ctx := tool.NewContext(context.Background())
	ctx.RequestID = "req-1"
	ctx.ToolUseID = "use-1"

	if ctx.RequestID != "req-1" {
		t.Errorf("RequestID = %q", ctx.RequestID)
	}
	if ctx.ToolUseID != "use-1" {
		t.Errorf("ToolUseID = %q", ctx.ToolUseID)
	}
}

func TestContext_Metadata(t *testing.T) {
	ctx := tool.Background()
	ctx.Set("key1", "value1")
	ctx.Set("key2", 42)

	v, ok := ctx.Get("key1")
	if !ok || v != "value1" {
		t.Errorf("Get(key1) = %v, %v", v, ok)
	}

	v, ok = ctx.Get("key2")
	if !ok || v != 42 {
		t.Errorf("Get(key2) = %v, %v", v, ok)
	}

	_, ok = ctx.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}

	meta := ctx.Metadata()
	if len(meta) != 2 {
		t.Errorf("Metadata() length = %d, want 2", len(meta))
	}
}

func TestContext_WithTimeout(t *testing.T) {
	ctx := tool.Background()
	tctx, cancel := ctx.WithTimeout(50 * time.Millisecond)
	defer cancel()

	select {
	case <-tctx.Done():
		// OK — timed out
	case <-time.After(1 * time.Second):
		t.Error("expected timeout within 50ms")
	}
}

func TestContext_WithCancel(t *testing.T) {
	ctx := tool.Background()
	cctx, cancel := ctx.WithCancel()

	cancel()

	if cctx.Err() == nil {
		t.Error("expected canceled context")
	}
}

// --- Result tests ---

func TestTextResult(t *testing.T) {
	r := tool.TextResult("hello world")
	if r.Content != "hello world" {
		t.Errorf("Content = %q", r.Content)
	}
	if r.IsError {
		t.Error("should not be error")
	}
	if r.Text() != "hello world" {
		t.Errorf("Text() = %q", r.Text())
	}
}

func TestErrorResult(t *testing.T) {
	r := tool.ErrorResult("something failed")
	if r.Content != "something failed" {
		t.Errorf("Content = %q", r.Content)
	}
	if !r.IsError {
		t.Error("should be error")
	}
}

func TestJSONResult(t *testing.T) {
	data := map[string]int{"count": 42}
	r, err := tool.JSONResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Output == nil {
		t.Fatal("expected non-nil Output")
	}
	if r.Content == "" {
		t.Error("expected non-empty Content")
	}

	var out map[string]int
	if err := json.Unmarshal(r.Output, &out); err != nil {
		t.Fatal(err)
	}
	if out["count"] != 42 {
		t.Errorf("output count = %d", out["count"])
	}
}

func TestMustJSONResult(t *testing.T) {
	r := tool.MustJSONResult(SearchOutput{Items: []string{"a"}, Total: 1})
	if r.Output == nil {
		t.Fatal("expected non-nil Output")
	}
}

func TestResult_SetMeta(t *testing.T) {
	r := tool.TextResult("test")
	r.SetMeta("duration", "100ms")
	if r.Metadata["duration"] != "100ms" {
		t.Errorf("metadata = %v", r.Metadata)
	}
}

// --- Registry tests ---

func TestRegistry_ComponentLifecycle(t *testing.T) {
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("search", "Search", doSearch).AsCallable())

	if reg.Name() != "tool-registry" {
		t.Errorf("Name() = %q", reg.Name())
	}
	// Before Start the registry reports degraded.
	if h := reg.Health(context.Background()); h.Status != component.StatusDegraded {
		t.Errorf("pre-start health = %v, want degraded", h.Status)
	}
	if err := reg.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h := reg.Health(context.Background()); h.Status != component.StatusHealthy {
		t.Errorf("post-start health = %v, want healthy", h.Status)
	}
	if got := reg.Names(); len(got) != 1 || got[0] != "search" {
		t.Errorf("Names() = %v, want [search]", got)
	}
	if got := reg.ToolSpecs(); len(got) != 1 || got[0].Name != "search" {
		t.Errorf("ToolSpecs() = %+v", got)
	}
	if err := reg.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h := reg.Health(context.Background()); h.Status != component.StatusDegraded {
		t.Errorf("post-stop health = %v, want degraded", h.Status)
	}
}

func TestRegistry_RegisterNil(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(nil); err == nil {
		t.Fatal("expected error registering nil callable")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := tool.NewRegistry()
	st := tool.FromFunc("search", "Search", doSearch)

	if err := reg.Register(st.AsCallable()); err != nil {
		t.Fatal(err)
	}

	c, ok := reg.Get("search")
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if c.Definition().Name != "search" {
		t.Error("wrong tool returned")
	}
}

func TestRegistry_DuplicateName(t *testing.T) {
	reg := tool.NewRegistry()
	st := tool.FromFunc("search", "Search", doSearch)

	_ = reg.Register(st.AsCallable())
	err := reg.Register(st.AsCallable())
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("a", "Tool A", doSearch).AsCallable())
	mustReg(t, reg, tool.FromFunc("b", "Tool B", doSearch).AsCallable())

	defs := reg.List()
	if len(defs) != 2 {
		t.Errorf("expected 2 definitions, got %d", len(defs))
	}
}

func TestRegistry_Call(t *testing.T) {
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("search", "Search", doSearch).AsCallable())

	result, err := reg.Call(tool.Background(), "search", json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatal(err)
	}

	var out SearchOutput
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 {
		t.Errorf("expected total=2, got %d", out.Total)
	}
}

func TestRegistry_CallNotFound(t *testing.T) {
	reg := tool.NewRegistry()
	_, err := reg.Call(tool.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestRegistry_Search(t *testing.T) {
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("search_videos", "Search for videos", doSearch).AsCallable())
	mustReg(t, reg, tool.FromFunc("clip_media", "Clip media content", doSearch).AsCallable())
	mustReg(t, reg, tool.FromFunc("analyze", "Analyze content quality", doSearch).AsCallable())

	defs := reg.Search("search")
	if len(defs) != 1 {
		t.Errorf("expected 1 result for 'search', got %d", len(defs))
	}

	defs = reg.Search("content")
	if len(defs) != 2 {
		t.Errorf("expected 2 results for 'content', got %d", len(defs))
	}

	defs = reg.Search("nonexistent")
	if len(defs) != 0 {
		t.Errorf("expected 0 results, got %d", len(defs))
	}
}

func TestRegistry_CallBatch(t *testing.T) {
	readFn := func(ctx context.Context, in SearchInput) (SearchOutput, error) {
		return SearchOutput{Items: []string{"read"}, Total: 1}, nil
	}
	writeFn := func(ctx context.Context, in SearchInput) (SearchOutput, error) {
		return SearchOutput{Items: []string{"write"}, Total: 1}, nil
	}

	reg := tool.NewRegistry()

	readTool := tool.FromFunc("read_tool", "Reads data", readFn)
	readTool.Def.Envelope.Safety = tool.SafetyReadOnly
	mustReg(t, reg, readTool.AsCallable())
	mustReg(t, reg, tool.FromFunc("write_tool", "Writes data", writeFn).AsCallable())

	calls := []tool.BatchCall{
		{Name: "read_tool", ID: "c1", Input: json.RawMessage(`{"query":"a"}`)},
		{Name: "write_tool", ID: "c2", Input: json.RawMessage(`{"query":"b"}`)},
		{Name: "nonexistent", ID: "c3", Input: nil},
	}

	results := reg.CallBatch(tool.Background(), calls, tool.BatchOptions{Concurrency: 2})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Err != nil {
		t.Errorf("result[0] error: %v", results[0].Err)
	}
	if results[0].ID != "c1" {
		t.Errorf("result[0].ID = %q", results[0].ID)
	}

	if results[1].Err != nil {
		t.Errorf("result[1] error: %v", results[1].Err)
	}

	if results[2].Err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestRegistry_CallBatch_Empty(t *testing.T) {
	reg := tool.NewRegistry()
	results := reg.CallBatch(tool.Background(), nil, tool.BatchOptions{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestRegistry_CallBatch_FailFast(t *testing.T) {
	failFn := func(ctx context.Context, in SearchInput) (SearchOutput, error) {
		return SearchOutput{}, errors.New("boom")
	}
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("fail_tool", "always fails", failFn).AsCallable())

	calls := []tool.BatchCall{
		{Name: "fail_tool", ID: "c1", Input: json.RawMessage(`{"query":"a"}`)},
		{Name: "fail_tool", ID: "c2", Input: json.RawMessage(`{"query":"b"}`)},
		{Name: "fail_tool", ID: "c3", Input: json.RawMessage(`{"query":"c"}`)},
	}
	results := reg.CallBatch(tool.Background(), calls, tool.BatchOptions{Concurrency: 1, FailFast: true})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// With serial fail-fast the first call fails and later calls are stopped.
	if results[0].Err == nil {
		t.Fatal("expected first call to fail")
	}
	stopped := 0
	for _, r := range results[1:] {
		if r.Err != nil {
			stopped++
		}
	}
	if stopped == 0 {
		t.Fatal("fail-fast should stop at least one subsequent call")
	}
}

func TestRegistry_Filter(t *testing.T) {
	reg := tool.NewRegistry()

	mustReg(t, reg, tool.FromFunc("search", "Search", doSearch).
		WithAnnotations(tool.Annotations{Category: "discovery", Tags: []string{"search"}}).
		AsCallable())
	mustReg(t, reg, tool.FromFunc("clip", "Clip", doSearch).
		WithAnnotations(tool.Annotations{Category: "media"}).
		AsCallable())
	mustReg(t, reg, tool.FromFunc("other", "Other", doSearch).AsCallable())

	defs := reg.Filter(tool.WithCategory("discovery"))
	if len(defs) != 1 {
		t.Errorf("expected 1 discovery tool, got %d", len(defs))
	}

	defs = reg.Filter(tool.WithTags("search"))
	if len(defs) != 1 {
		t.Errorf("expected 1 tagged tool, got %d", len(defs))
	}
}

func TestAnnotations_ExecutionHint(t *testing.T) {
	st := tool.FromFunc("validate", "Validate input", doSearch).
		WithAnnotations(tool.Annotations{
			Category:      "forms",
			ExecutionHint: tool.ExecutionUI,
		})

	def := st.Definition()
	if def.Annotations.ExecutionHint.Resolved() != tool.ExecutionUI {
		t.Errorf("expected executionHint 'ui', got %q", def.Annotations.ExecutionHint)
	}
}

func TestRegistry_FilterByExecutionHint(t *testing.T) {
	reg := tool.NewRegistry()

	mustReg(t, reg, tool.FromFunc("validate", "Validate", doSearch).
		WithAnnotations(tool.Annotations{ExecutionHint: tool.ExecutionUI}).
		AsCallable())
	mustReg(t, reg, tool.FromFunc("process", "Process", doSearch).
		WithAnnotations(tool.Annotations{ExecutionHint: tool.ExecutionBackend}).
		AsCallable())
	mustReg(t, reg, tool.FromFunc("submit", "Submit", doSearch).
		WithAnnotations(tool.Annotations{ExecutionHint: tool.ExecutionHybrid}).
		AsCallable())
	mustReg(t, reg, tool.FromFunc("plain", "Plain", doSearch).AsCallable())

	defs := reg.Filter(tool.WithExecutionHint(tool.ExecutionUI))
	if len(defs) != 1 || defs[0].Name != "validate" {
		t.Errorf("expected 1 ui tool (validate), got %d", len(defs))
	}

	defs = reg.Filter(tool.WithExecutionHint(tool.ExecutionBackend))
	if len(defs) != 2 {
		t.Errorf("expected 2 backend tools (process + plain), got %d", len(defs))
	}

	defs = reg.Filter(tool.WithExecutionHint(tool.ExecutionHybrid))
	if len(defs) != 1 || defs[0].Name != "submit" {
		t.Errorf("expected 1 hybrid tool (submit), got %d", len(defs))
	}
}

// --- Provider bridge test ---

func TestAsProvider(t *testing.T) {
	st := tool.FromFunc("search", "Search", doSearch)
	p := st.AsProvider()

	if p.Name() != "search" {
		t.Errorf("expected name 'search', got %q", p.Name())
	}

	out, err := p.Execute(context.Background(), SearchInput{Query: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 {
		t.Errorf("expected total=2, got %d", out.Total)
	}
}

// mustReg registers t on reg, failing the test on error.
func mustReg(tb testing.TB, reg *tool.Registry, c tool.Callable) {
	tb.Helper()
	if err := reg.Register(c); err != nil {
		tb.Fatalf("register: %v", err)
	}
}
