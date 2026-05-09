package prompt

import (
	"fmt"
	"strings"
)

type section struct {
	name    string
	content string
	err     error
}

// Builder composes a prompt from named sections.
type Builder struct {
	sections  []section
	separator string
}

// NewBuilder creates a prompt builder.
func NewBuilder() *Builder { return &Builder{separator: "\n\n"} }

// Section appends a static section.
func (b *Builder) Section(name, content string) *Builder {
	b.sections = append(b.sections, section{name: name, content: content})
	return b
}

// SectionIf appends a section only when condition is true.
func (b *Builder) SectionIf(condition bool, name, content string) *Builder {
	if condition {
		b.sections = append(b.sections, section{name: name, content: content})
	}
	return b
}

// SectionTemplate renders a Template and appends it as a section.
func (b *Builder) SectionTemplate(name string, tmpl *Template, data any) *Builder {
	content, err := tmpl.Render(data)
	b.sections = append(b.sections, section{name: name, content: content, err: err})
	return b
}

// Separator sets the section separator.
func (b *Builder) Separator(sep string) *Builder { b.separator = sep; return b }

// Build returns the composed prompt.
func (b *Builder) Build() (string, error) {
	parts := make([]string, 0, len(b.sections))
	for _, s := range b.sections {
		if s.err != nil {
			return "", fmt.Errorf("prompt: build section %q: %w", s.name, s.err)
		}
		parts = append(parts, s.content)
	}
	return strings.Join(parts, b.separator), nil
}
