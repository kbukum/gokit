package prompt_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/ai/prompt"
)

func TestRenderMustacheTemplate(t *testing.T) {
	pt, err := prompt.NewTemplate("test", "Hello, {{ name }}!")
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}
	got, err := pt.Render(map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "Hello, World!" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderRejectsUndeclaredVariables(t *testing.T) {
	_, err := prompt.Render("Hello, {{name}} {{missing}}", map[string]any{"name": "Ada"})
	if err == nil || !strings.Contains(err.Error(), "missing template variables: missing") {
		t.Fatalf("expected missing variable error, got %v", err)
	}
}

func TestNewTemplateInvalidSyntax(t *testing.T) {
	if _, err := prompt.NewTemplate("bad", "{{.Name}}"); err == nil {
		t.Fatal("expected invalid placeholder error")
	}
}

func TestValidateFindsUndeclaredAndUnused(t *testing.T) {
	issues := prompt.Validate(prompt.PromptTemplate{
		Template:  "Hi {{name}} from {{city}}",
		Variables: []prompt.VariableDecl{{Name: "name", Required: true}, {Name: "unused"}},
	})
	if len(issues) != 2 {
		t.Fatalf("issues=%+v", issues)
	}
	if issues[0].Name != "city" || issues[0].Kind != "undeclared" || issues[1].Name != "unused" || issues[1].Kind != "unused" {
		t.Fatalf("issues=%+v", issues)
	}
}

func TestRegistryLookupLatestAndVersions(t *testing.T) {
	reg := prompt.NewRegistry()
	for _, version := range []string{"1.0.0", "1.2.0", "1.1.0"} {
		if err := reg.Register("summarize", version, "Summarize {{input}}"); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	versions := reg.Versions("summarize")
	if strings.Join(versions, ",") != "1.0.0,1.1.0,1.2.0" {
		t.Fatalf("versions=%v", versions)
	}
	latest, ok := reg.LookupLatest("summarize")
	if !ok || latest.Version != "1.2.0" {
		t.Fatalf("latest=%+v ok=%v", latest, ok)
	}
	if len(reg.List()) != 3 {
		t.Fatalf("list=%+v", reg.List())
	}
}

func TestRegistrySemverPrereleaseOrdering(t *testing.T) {
	reg := prompt.NewRegistry()
	for _, version := range []string{"1.0.0", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-beta", "1.0.0-rc.1"} {
		if err := reg.Register("summarize", version, "Summarize {{input}}"); err != nil {
			t.Fatalf("Register(%s): %v", version, err)
		}
	}
	versions := reg.Versions("summarize")
	if strings.Join(versions, ",") != "1.0.0-beta,1.0.0-beta.2,1.0.0-beta.11,1.0.0-rc.1,1.0.0" {
		t.Fatalf("versions=%v", versions)
	}
	latest, ok := reg.LookupLatest("summarize")
	if !ok || latest.Version != "1.0.0" {
		t.Fatalf("latest=%+v ok=%v", latest, ok)
	}
}

func TestRenderToMessageAndBuilder(t *testing.T) {
	pt := mustTmpl(t, "sys", "You are {{role}}.")
	msg, err := pt.RenderToMessage(map[string]any{"role": "helpful"})
	if err != nil {
		t.Fatalf("RenderToMessage: %v", err)
	}
	sysMsg, ok := msg.(chat.SystemMessage)
	if !ok || sysMsg.Content != "You are helpful." {
		t.Fatalf("msg=%T %+v", msg, msg)
	}
	got, err := prompt.NewBuilder().Section("a", "A").SectionTemplate("b", pt, map[string]any{"role": "kind"}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != "A\n\nYou are kind." {
		t.Fatalf("got %q", got)
	}
}

func TestBuilderPropagatesTemplateError(t *testing.T) {
	pt := mustTmpl(t, "bad", "{{missing}}")
	_, err := prompt.NewBuilder().SectionTemplate("broken", pt, map[string]any{}).Build()
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected section error, got %v", err)
	}
}

func mustTmpl(tb testing.TB, name, src string) *prompt.Template {
	tb.Helper()
	pt, err := prompt.NewTemplate(name, src)
	if err != nil {
		tb.Fatalf("NewTemplate(%q): %v", name, err)
	}
	return pt
}

func TestTemplateOptionsAndVariableInputs(t *testing.T) {
	pt, err := prompt.NewTemplate("opts", "{{Name}}/{{kind}}", prompt.WithDescription("desc"), prompt.WithOutputSchema(map[string]any{"type": "object"}), prompt.WithVariables(prompt.VariableDecl{Name: "Name"}, prompt.VariableDecl{Name: "kind"}))
	if err != nil {
		t.Fatal(err)
	}
	if pt.Description != "desc" || pt.OutputSchema["type"] != "object" || len(pt.Variables) != 2 {
		t.Fatalf("template=%+v", pt)
	}
	got, err := pt.Render(struct{ Name string }{Name: "Ada"})
	if err == nil || !strings.Contains(err.Error(), "kind") || got != "" {
		t.Fatalf("expected missing kind, got %q err %v", got, err)
	}
	got, err = pt.Render(map[string]string{"Name": "Ada", "kind": "math"})
	if err != nil || got != "Ada/math" {
		t.Fatalf("got %q err %v", got, err)
	}
	var nilMap map[string]any
	if _, err := pt.Render(nilMap); err == nil {
		t.Fatal("expected nil map missing variables")
	}
	if _, err := pt.Render(42); err == nil {
		t.Fatal("expected unsupported variables error")
	}
}

func TestRenderSyntaxAndNilTemplateErrors(t *testing.T) {
	var pt *prompt.Template
	if _, err := pt.Render(nil); err == nil {
		t.Fatal("expected nil template error")
	}
	if _, err := prompt.Render("hello }}", map[string]any{}); err == nil {
		t.Fatal("expected unopened placeholder error")
	}
	if _, err := prompt.Render("{{missing}} {{missing}}", map[string]any{}); err == nil || strings.Count(err.Error(), "missing template variables: missing") != 1 {
		t.Fatalf("expected unique missing variable, got %v", err)
	}
}

func TestBuilderConditionAndSeparator(t *testing.T) {
	got, err := prompt.NewBuilder().Separator("|").Section("a", "A").SectionIf(false, "b", "B").SectionIf(true, "c", "C").Build()
	if err != nil {
		t.Fatal(err)
	}
	if got != "A|C" {
		t.Fatalf("got %q", got)
	}
}

func TestRegistryFailuresAndMissingLookups(t *testing.T) {
	var nilReg *prompt.Registry
	if err := nilReg.Register("x", "1.0.0", "{{x}}"); err == nil {
		t.Fatal("expected nil registry error")
	}
	if _, ok := nilReg.Lookup("x", "1.0.0"); ok {
		t.Fatal("nil registry lookup should miss")
	}
	if _, ok := nilReg.LookupLatest("x"); ok {
		t.Fatal("nil registry latest should miss")
	}
	reg := prompt.NewRegistry()
	if err := reg.Register("", "1.0.0", "x"); err == nil {
		t.Fatal("expected missing name error")
	}
	if err := reg.Register("x", "1.0.0", "{{.bad}}"); err == nil {
		t.Fatal("expected invalid template error")
	}
	if err := reg.Register("x", "1.0.0", "{{x}}"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register("x", "1.0.0", "{{x}}"); err == nil {
		t.Fatal("expected duplicate error")
	}
	if _, ok := reg.Lookup("x", "2.0.0"); ok {
		t.Fatal("lookup should miss")
	}
	if len(nilReg.List()) != 0 || len(nilReg.Versions("x")) != 0 {
		t.Fatal("nil registry should be empty")
	}
}
