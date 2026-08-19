package util

import (
	"sort"
	"strings"
)

// DynamicTemplate is a parsed "{{var}}" template with an open set of named variables
// resolved at render time. Unlike Template, whose placeholders are a fixed typed set
// known at compile time, a DynamicTemplate carries data-driven variable names looked
// up against a caller-supplied function. Parsing is lenient: names may be padded with
// whitespace ("{{ name }}" equals "{{name}}"), and any run that is not a well-formed
// placeholder is preserved verbatim. Use it for prompt-style templates.
type DynamicTemplate struct {
	parts []dynamicPart
}

type dynamicPartKind int

const (
	dynamicLiteral dynamicPartKind = iota
	dynamicVariable
)

type dynamicPart struct {
	kind  dynamicPartKind
	value string
}

// ParseDynamicTemplate parses template. Parsing never fails: malformed "{{"/"}}" runs
// and invalid names are kept as literal text.
func ParseDynamicTemplate(template string) DynamicTemplate {
	var parts []dynamicPart
	rest := template
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			break
		}
		before := rest[:start]
		after := rest[start+2:]
		if before != "" {
			parts = append(parts, dynamicPart{kind: dynamicLiteral, value: before})
		}
		end := strings.Index(after, "}}")
		if end < 0 {
			parts = append(parts, dynamicPart{kind: dynamicLiteral, value: "{{"})
			rest = after
			continue
		}
		name := strings.TrimSpace(after[:end])
		if isValidVariableName(name) {
			parts = append(parts, dynamicPart{kind: dynamicVariable, value: name})
		} else {
			parts = append(parts, dynamicPart{kind: dynamicLiteral, value: "{{" + after[:end] + "}}"})
		}
		rest = after[end+2:]
	}
	if rest != "" {
		parts = append(parts, dynamicPart{kind: dynamicLiteral, value: rest})
	}
	return DynamicTemplate{parts: parts}
}

// Variables returns the sorted, de-duplicated set of variable names the template
// references.
func (t DynamicTemplate) Variables() []string {
	seen := make(map[string]struct{})
	for _, part := range t.parts {
		if part.kind == dynamicVariable {
			seen[part.value] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Render resolves each variable through lookup, which returns the value and whether
// it was found. A variable with no value yields a TemplateError of kind
// TemplateErrorMissingVariable.
func (t DynamicTemplate) Render(lookup func(name string) (string, bool)) (string, error) {
	var builder strings.Builder
	for _, part := range t.parts {
		switch part.kind {
		case dynamicLiteral:
			builder.WriteString(part.value)
		case dynamicVariable:
			value, ok := lookup(part.value)
			if !ok {
				return "", TemplateError{Kind: TemplateErrorMissingVariable, Detail: part.value}
			}
			builder.WriteString(value)
		}
	}
	return builder.String(), nil
}

func isValidVariableName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c == '_' || c == '-' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}
