package util

import (
	"errors"
	"testing"
)

type testToken int

const (
	tokenName testToken = iota
	tokenArgs
)

func (t testToken) Token() string {
	switch t {
	case tokenName:
		return "name"
	case tokenArgs:
		return "args"
	default:
		return ""
	}
}

var testTokens = []testToken{tokenName, tokenArgs}

func TestTemplateParsesKnownPlaceholders(t *testing.T) {
	t.Parallel()
	tpl, err := ParseTemplate("cargo {name} {args}", testTokens)
	if err != nil {
		t.Fatal(err)
	}
	if !tpl.Contains(tokenName) || !tpl.Contains(tokenArgs) {
		t.Fatal("expected both placeholders present")
	}
	if len(tpl.Parts()) != 4 {
		t.Fatalf("parts = %d, want 4", len(tpl.Parts()))
	}
}

func TestTemplateRejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := ParseTemplate("{project.root}", testTokens)
	want := TemplateError{Kind: TemplateErrorUnknownPlaceholder, Detail: "project.root"}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestTemplateRejectsUnclosed(t *testing.T) {
	t.Parallel()
	_, err := ParseTemplate("cargo {name", testTokens)
	want := TemplateError{Kind: TemplateErrorUnclosedPlaceholder, Detail: "cargo {name"}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemplateRejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := ParseTemplate("{}", testTokens)
	if !errors.Is(err, TemplateError{Kind: TemplateErrorEmptyPlaceholder}) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemplateRejectsUnmatchedClosing(t *testing.T) {
	t.Parallel()
	_, err := ParseTemplate("cargo } {name}", testTokens)
	want := TemplateError{Kind: TemplateErrorUnmatchedClosingBrace, Detail: "cargo } {name}"}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemplateRenderWith(t *testing.T) {
	t.Parallel()
	tpl, err := ParseTemplate("{name}:{args}", testTokens)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tpl.RenderWith(func(p testToken) (string, error) {
		switch p {
		case tokenName:
			return "build", nil
		case tokenArgs:
			return "--all", nil
		default:
			return "", nil
		}
	})
	if err != nil || out != "build:--all" {
		t.Fatalf("render = (%q, %v)", out, err)
	}
}

func TestDynamicTemplateRenders(t *testing.T) {
	t.Parallel()
	tpl := ParseDynamicTemplate("hi {{ name }}, {{count}} items")
	out, err := tpl.Render(func(name string) (string, bool) {
		switch name {
		case "name":
			return "ada", true
		case "count":
			return "3", true
		default:
			return "", false
		}
	})
	if err != nil || out != "hi ada, 3 items" {
		t.Fatalf("render = (%q, %v)", out, err)
	}
}

func TestDynamicTemplateVariables(t *testing.T) {
	t.Parallel()
	tpl := ParseDynamicTemplate("{{a}}{{ b }}{{a}}")
	vars := tpl.Variables()
	if len(vars) != 2 || vars[0] != "a" || vars[1] != "b" {
		t.Fatalf("variables = %v", vars)
	}
}

func TestDynamicTemplateMissingVariable(t *testing.T) {
	t.Parallel()
	tpl := ParseDynamicTemplate("{{x}}")
	_, err := tpl.Render(func(string) (string, bool) { return "", false })
	if !errors.Is(err, TemplateError{Kind: TemplateErrorMissingVariable, Detail: "x"}) {
		t.Fatalf("err = %v", err)
	}
}

func TestDynamicTemplateMalformedIsLiteral(t *testing.T) {
	t.Parallel()
	tpl := ParseDynamicTemplate("a {{ bad name }} b {{")
	out, err := tpl.Render(func(string) (string, bool) { return "", false })
	if err != nil || out != "a {{ bad name }} b {{" {
		t.Fatalf("render = (%q, %v)", out, err)
	}
}
