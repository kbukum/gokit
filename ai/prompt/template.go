package prompt

import (
	"fmt"

	"github.com/kbukum/gokit/schema"
)

// VariableDecl declares a mustache-style template variable.
type VariableDecl struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// PromptTemplate is the canonical prompt template record stored in registries.
type PromptTemplate struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Template     string         `json:"template"`
	Variables    []VariableDecl `json:"variables,omitempty"`
	OutputSchema schema.JSON    `json:"output_schema,omitempty"`
	Description  string         `json:"description,omitempty"`
}

// ValidationIssue identifies a declared-but-unused or used-but-undeclared variable.
type ValidationIssue struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Template wraps a canonical mustache-style prompt template.
type Template struct {
	PromptTemplate
}

// Option configures a Template.
type Option func(*Template)

// WithDescription sets template description metadata.
func WithDescription(description string) Option {
	return func(t *Template) { t.Description = description }
}

// WithOutputSchema sets the JSON Schema 2020-12 schema for expected output.
func WithOutputSchema(output schema.JSON) Option {
	return func(t *Template) { t.OutputSchema = output }
}

// WithVariables declares the variables expected by the template.
func WithVariables(vars ...VariableDecl) Option {
	return func(t *Template) { t.Variables = append([]VariableDecl(nil), vars...) }
}

// NewTemplate validates a named mustache-style template string.
func NewTemplate(name, tmpl string, opts ...Option) (*Template, error) {
	if err := validateSyntax(tmpl); err != nil {
		return nil, fmt.Errorf("prompt: parse template %q: %w", name, err)
	}
	pt := &Template{PromptTemplate: PromptTemplate{Name: name, Template: tmpl}}
	for _, opt := range opts {
		opt(pt)
	}
	return pt, nil
}
