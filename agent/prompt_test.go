package agent_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/llm"
)

// --- PromptTemplate Tests ---

func TestNew_ValidTemplate(t *testing.T) {
	pt, err := agent.NewPromptTemplate("test", "Hello, {{.Name}}!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := pt.Render(struct{ Name string }{"World"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "Hello, World!" {
		t.Errorf("got %q, want %q", got, "Hello, World!")
	}
}

func TestNew_InvalidTemplate(t *testing.T) {
	_, err := agent.NewPromptTemplate("bad", "{{.Unclosed")
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
	if !strings.Contains(err.Error(), "parse template") {
		t.Errorf("error should mention parse template, got: %v", err)
	}
}

func TestNew_InvalidReturnsError(t *testing.T) {
	if _, err := agent.NewPromptTemplate("bad", "{{.Unclosed"); err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestMustNew_Valid(t *testing.T) {
	pt := mustTmpl(t, "ok", "static prompt")
	got, err := pt.Render(nil)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "static prompt" {
		t.Errorf("got %q, want %q", got, "static prompt")
	}
}

func TestRender_Error(t *testing.T) {
	pt := mustTmpl(t, "err", "{{.Missing.Field}}")
	_, err := pt.Render(struct{}{})
	if err == nil {
		t.Fatal("expected render error for missing field")
	}
	if !strings.Contains(err.Error(), "render template") {
		t.Errorf("error should mention render template, got: %v", err)
	}
}

func TestRenderToMessage(t *testing.T) {
	pt := mustTmpl(t, "sys", "You are {{.Role}}.")
	msg, err := pt.RenderToMessage(struct{ Role string }{"a helpful assistant"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sysMsg, ok := msg.(llm.SystemMessage)
	if !ok {
		t.Fatalf("expected SystemMessage, got %T", msg)
	}
	if sysMsg.Content != "You are a helpful assistant." {
		t.Errorf("got %q, want %q", sysMsg.Content, "You are a helpful assistant.")
	}
	if sysMsg.Role() != "system" {
		t.Errorf("role = %q, want %q", sysMsg.Role(), "system")
	}
}

func TestRenderToMessage_Error(t *testing.T) {
	pt := mustTmpl(t, "err", "{{.Missing.Field}}")
	_, err := pt.RenderToMessage(struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Built-in Function Tests ---

func TestFunc_Join(t *testing.T) {
	pt := mustTmpl(t, "join", `{{join .Items ", "}}`)
	got, err := pt.Render(struct{ Items []string }{[]string{"a", "b", "c"}})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "a, b, c" {
		t.Errorf("got %q, want %q", got, "a, b, c")
	}
}

func TestFunc_Upper(t *testing.T) {
	pt := mustTmpl(t, "upper", `{{upper .Text}}`)
	got, err := pt.Render(struct{ Text string }{"hello"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "HELLO" {
		t.Errorf("got %q, want %q", got, "HELLO")
	}
}

func TestFunc_Lower(t *testing.T) {
	pt := mustTmpl(t, "lower", `{{lower .Text}}`)
	got, err := pt.Render(struct{ Text string }{"HELLO"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFunc_Trim(t *testing.T) {
	pt := mustTmpl(t, "trim", `{{trim .Text}}`)
	got, err := pt.Render(struct{ Text string }{"  hello  "})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFunc_Indent(t *testing.T) {
	pt := mustTmpl(t, "indent", `{{indent 4 .Text}}`)
	got, err := pt.Render(struct{ Text string }{"line1\nline2\nline3"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	want := "    line1\n    line2\n    line3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFunc_Indent_PreservesBlankLines(t *testing.T) {
	pt := mustTmpl(t, "indent-blank", `{{indent 2 .Text}}`)
	got, err := pt.Render(struct{ Text string }{"a\n\nb"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	want := "  a\n\n  b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFunc_ToJSON(t *testing.T) {
	pt := mustTmpl(t, "json", `{{toJSON .Data}}`)
	data := struct {
		Data map[string]string
	}{
		Data: map[string]string{"key": "value"},
	}
	got, err := pt.Render(data)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != `{"key":"value"}` {
		t.Errorf("got %q, want %q", got, `{"key":"value"}`)
	}
}

func TestFunc_Default_UsesValue(t *testing.T) {
	pt := mustTmpl(t, "def-val", `{{default "fallback" .Name}}`)
	got, err := pt.Render(struct{ Name string }{"Alice"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "Alice" {
		t.Errorf("got %q, want %q", got, "Alice")
	}
}

func TestFunc_Default_UsesFallback(t *testing.T) {
	pt := mustTmpl(t, "def-fb", `{{default "fallback" .Name}}`)
	got, err := pt.Render(struct{ Name string }{""})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("got %q, want %q", got, "fallback")
	}
}

func TestFunc_Default_NilUsesFallback(t *testing.T) {
	pt := mustTmpl(t, "def-nil", `{{default "none" .Val}}`)
	got, err := pt.Render(map[string]any{"Val": nil})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if got != "none" {
		t.Errorf("got %q, want %q", got, "none")
	}
}

// --- PromptBuilder Tests ---

func TestPromptBuilder_Basic(t *testing.T) {
	got, err := agent.NewPromptBuilder().
		Section("role", "You are a helpful assistant.").
		Section("rules", "Always be concise.").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "You are a helpful assistant.\n\nAlways be concise."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromptBuilder_CustomSeparator(t *testing.T) {
	got, err := agent.NewPromptBuilder().
		Separator("\n---\n").
		Section("a", "First").
		Section("b", "Second").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "First\n---\nSecond"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromptBuilder_SectionIf_True(t *testing.T) {
	got, err := agent.NewPromptBuilder().
		Section("base", "Base prompt.").
		SectionIf(true, "extra", "Extra context.").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Extra context.") {
		t.Errorf("expected extra section, got %q", got)
	}
}

func TestPromptBuilder_SectionIf_False(t *testing.T) {
	got, err := agent.NewPromptBuilder().
		Section("base", "Base prompt.").
		SectionIf(false, "extra", "Extra context.").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "Extra context.") {
		t.Errorf("should not contain extra section, got %q", got)
	}
}

func TestPromptBuilder_SectionTemplate(t *testing.T) {
	tmpl := mustTmpl(t, "ctx", "Current page: {{.Page}}")
	got, err := agent.NewPromptBuilder().
		Section("role", "You are a page assistant.").
		SectionTemplate("context", tmpl, struct{ Page string }{"Home"}).
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "You are a page assistant.\n\nCurrent page: Home"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromptBuilder_SectionTemplate_Error(t *testing.T) {
	tmpl := mustTmpl(t, "bad", "{{.Missing.Deep}}")
	_, err := agent.NewPromptBuilder().
		SectionTemplate("broken", tmpl, struct{}{}).
		Build()
	if err == nil {
		t.Fatal("expected error from broken template section")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should reference section name, got: %v", err)
	}
}

func TestPromptBuilder_Empty(t *testing.T) {
	got, err := agent.NewPromptBuilder().Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestPromptBuilder_Build(t *testing.T) {
	got, err := agent.NewPromptBuilder().
		Section("a", "hello").
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestPromptBuilder_Build_PropagatesError(t *testing.T) {
	tmpl := mustTmpl(t, "bad", "{{.Missing.Deep}}")
	_, err := agent.NewPromptBuilder().
		SectionTemplate("broken", tmpl, struct{}{}).
		Build()
	if err == nil {
		t.Fatal("expected Build to propagate template render error")
	}
}

// --- Integration: Config with SystemPromptTemplate ---

func TestConfig_SystemPromptTemplate(t *testing.T) {
	tmpl := mustTmpl(t, "sys", "You help with {{.Topic}}. Be {{.Style}}.")
	data := struct {
		Topic string
		Style string
	}{"math", "concise"}

	got, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	want := "You help with math. Be concise."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromptTemplate_ComplexExample(t *testing.T) {
	tmpl := mustTmpl(t, "complex", `You are a {{.Role}} assistant.
{{- if .Tools}}

Available tools:
{{- range .Tools}}
- {{.}}
{{- end}}
{{- end}}

Rules:
{{- range .Rules}}
- {{.}}
{{- end}}`)

	data := struct {
		Role  string
		Tools []string
		Rules []string
	}{
		Role:  "coding",
		Tools: []string{"search", "edit", "run"},
		Rules: []string{"Be concise", "Show examples"},
	}

	got, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if !strings.Contains(got, "coding assistant") {
		t.Errorf("should contain role, got %q", got)
	}
	if !strings.Contains(got, "- search") {
		t.Errorf("should contain tools, got %q", got)
	}
	if !strings.Contains(got, "- Be concise") {
		t.Errorf("should contain rules, got %q", got)
	}
}

// mustTmpl parses a template, failing the test on error.
func mustTmpl(tb testing.TB, name, src string) *agent.PromptTemplate {
tb.Helper()
pt, err := agent.NewPromptTemplate(name, src)
if err != nil {
tb.Fatalf("NewPromptTemplate(%q): %v", name, err)
}
return pt
}
