package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/kbukum/gokit/llm"
)

// PromptTemplate wraps a Go text/template with built-in helper functions
// useful for composing LLM system prompts. Templates use standard Go
// template syntax (e.g. {{.Field}}, {{if .Cond}}...{{end}}).
type PromptTemplate struct {
	tmpl *template.Template
}

// builtinFuncs returns the template function map available in every PromptTemplate.
var builtinFuncs = template.FuncMap{
	// join concatenates string slices with a separator.
	"join": strings.Join,
	// upper converts a string to uppercase.
	"upper": strings.ToUpper,
	// lower converts a string to lowercase.
	"lower": strings.ToLower,
	// trim removes leading and trailing whitespace.
	"trim": strings.TrimSpace,
	// indent prefixes every line with n spaces.
	"indent": func(n int, s string) string {
		pad := strings.Repeat(" ", n)
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			if line != "" {
				lines[i] = pad + line
			}
		}
		return strings.Join(lines, "\n")
	},
	// toJSON marshals a value to a JSON string.
	"toJSON": func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("toJSON: %w", err)
		}
		return string(b), nil
	},
	// default returns the fallback value if the given value is empty/zero.
	"default": func(fallback, val any) any {
		if val == nil {
			return fallback
		}
		if s, ok := val.(string); ok && s == "" {
			return fallback
		}
		return val
	},
}

// NewPromptTemplate parses a named template string and returns a PromptTemplate.
// The template has access to built-in functions: join, upper, lower,
// trim, indent, toJSON, and default.
func NewPromptTemplate(name, tmpl string) (*PromptTemplate, error) {
	t, err := template.New(name).Funcs(builtinFuncs).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("agent: parse template %q: %w", name, err)
	}
	return &PromptTemplate{tmpl: t}, nil
}

// Render executes the template with the given data and returns the result.
func (pt *PromptTemplate) Render(data any) (string, error) {
	var buf bytes.Buffer
	if err := pt.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("agent: render template %q: %w", pt.tmpl.Name(), err)
	}
	return buf.String(), nil
}

// RenderToMessage renders the template and wraps the result as an llm.SystemMessage.
func (pt *PromptTemplate) RenderToMessage(data any) (llm.Message, error) {
	content, err := pt.Render(data)
	if err != nil {
		return nil, err
	}
	return llm.System(content), nil
}

// --- PromptBuilder ---

// section is an internal representation of a prompt section.
type section struct {
	name    string
	content string
	err     error
}

// PromptBuilder composes a system prompt from named sections.
// Sections are joined with a configurable separator (default: "\n\n").
type PromptBuilder struct {
	sections  []section
	separator string
}

// NewPromptBuilder creates a new PromptBuilder with default settings.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		separator: "\n\n",
	}
}

// Section appends a named static text section.
func (b *PromptBuilder) Section(name, content string) *PromptBuilder {
	b.sections = append(b.sections, section{name: name, content: content})
	return b
}

// SectionIf appends a section only when condition is true.
func (b *PromptBuilder) SectionIf(condition bool, name, content string) *PromptBuilder {
	if condition {
		b.sections = append(b.sections, section{name: name, content: content})
	}
	return b
}

// SectionTemplate renders a PromptTemplate and appends the result as a section.
// Any render error is deferred until Build is called.
func (b *PromptBuilder) SectionTemplate(name string, tmpl *PromptTemplate, data any) *PromptBuilder {
	content, err := tmpl.Render(data)
	b.sections = append(b.sections, section{name: name, content: content, err: err})
	return b
}

// Separator sets the string used to join sections (default: "\n\n").
func (b *PromptBuilder) Separator(sep string) *PromptBuilder {
	b.separator = sep
	return b
}

// Build joins all sections and returns the composed prompt.
// Returns an error if any SectionTemplate failed to render.
func (b *PromptBuilder) Build() (string, error) {
	parts := make([]string, 0, len(b.sections))
	for _, s := range b.sections {
		if s.err != nil {
			return "", fmt.Errorf("agent: build prompt section %q: %w", s.name, s.err)
		}
		parts = append(parts, s.content)
	}
	return strings.Join(parts, b.separator), nil
}
