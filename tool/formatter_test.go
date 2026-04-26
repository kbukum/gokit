package tool_test

import (
	"testing"

	"github.com/kbukum/gokit/tool"
)

func TestTruncateFormatter(t *testing.T) {
	f := tool.TruncateFormatter(20)
	r := &tool.Result{Content: "This is a long string that exceeds twenty characters"}
	out, err := f.Format("test", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 80 { // truncated content + suffix
		t.Errorf("expected truncated output, got %d chars", len(out))
	}
	if out[:20] != "This is a long strin" {
		t.Errorf("unexpected prefix: %q", out[:20])
	}

	// Short content passes through.
	r2 := &tool.Result{Content: "short"}
	out2, _ := f.Format("test", r2)
	if out2 != "short" {
		t.Errorf("expected 'short', got %q", out2)
	}
}

func TestSummaryHeaderFormatter(t *testing.T) {
	f := tool.SummaryHeaderFormatter()

	ok := &tool.Result{Content: "data"}
	out, _ := f.Format("search", ok)
	if out != "[search OK]\ndata" {
		t.Errorf("unexpected: %q", out)
	}

	errResult := &tool.Result{Content: "oops", IsError: true}
	out2, _ := f.Format("search", errResult)
	if out2 != "[search ERROR]\noops" {
		t.Errorf("unexpected: %q", out2)
	}
}

func TestMarkdownTableFormatter(t *testing.T) {
	data := `[{"name":"Alice","age":30},{"name":"Bob","age":25}]`
	r := &tool.Result{Content: data, Output: []byte(data)}
	out, err := tool.MarkdownTableFormatter.Format("list_users", r)
	if err != nil {
		t.Fatal(err)
	}
	if out == data {
		t.Error("expected markdown table, got raw JSON")
	}
	// Check table structure.
	if !contains(out, "| name") || !contains(out, "| ---") || !contains(out, "Alice") {
		t.Errorf("unexpected table:\n%s", out)
	}
}

func TestMarkdownTableFormatter_NonArray(t *testing.T) {
	r := &tool.Result{Content: `{"key":"value"}`}
	out, err := tool.MarkdownTableFormatter.Format("test", r)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"key":"value"}` {
		t.Errorf("non-array should pass through, got %q", out)
	}
}

func TestChainFormatters(t *testing.T) {
	chain := tool.ChainFormatters(
		tool.SummaryHeaderFormatter(),
		tool.TruncateFormatter(30),
	)
	r := &tool.Result{Content: "some data here that is moderately long"}
	out, err := chain.Format("mytool", r)
	if err != nil {
		t.Fatal(err)
	}
	// SummaryHeader adds "[mytool OK]\n" prefix, then truncate caps at 30.
	if len(out) > 100 {
		t.Errorf("chain should have truncated, got %d chars", len(out))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
