package util

import "strings"

// Placeholder is a typed template token. Implementations are comparable and expose
// their user-facing token name (without braces) via Token.
type Placeholder interface {
	comparable
	Token() string
}

// TemplatePartKind distinguishes a literal template part from a placeholder part.
type TemplatePartKind int

const (
	// TemplatePartLiteral is verbatim text.
	TemplatePartLiteral TemplatePartKind = iota
	// TemplatePartPlaceholder is a resolved placeholder token.
	TemplatePartPlaceholder
)

// TemplatePart is one parsed part of a typed Template: either literal text or a
// placeholder. When Kind is TemplatePartLiteral, Literal holds the text; when it is
// TemplatePartPlaceholder, Placeholder holds the token.
type TemplatePart[P Placeholder] struct {
	Kind        TemplatePartKind
	Literal     string
	Placeholder P
}

// Template is a parsed template string validated against a fixed, typed set of
// placeholders known at compile time. Unknown placeholders are rejected at parse
// time, so rendering only ever sees tokens the caller declared.
type Template[P Placeholder] struct {
	parts []TemplatePart[P]
}

// ParseTemplate parses value against the allowed placeholders, rejecting unknown or
// malformed placeholders and unmatched braces.
func ParseTemplate[P Placeholder](value string, placeholders []P) (Template[P], error) {
	var parts []TemplatePart[P]
	remaining := value

	for {
		start := strings.IndexByte(remaining, '{')
		if start < 0 {
			break
		}
		if start > 0 {
			literal, err := literalPart[P](value, remaining[:start])
			if err != nil {
				return Template[P]{}, err
			}
			parts = append(parts, literal)
		}
		afterOpen := remaining[start+1:]
		end := strings.IndexByte(afterOpen, '}')
		if end < 0 {
			return Template[P]{}, TemplateError{Kind: TemplateErrorUnclosedPlaceholder, Detail: value}
		}
		token := afterOpen[:end]
		placeholder, err := matchPlaceholder(token, placeholders)
		if err != nil {
			return Template[P]{}, err
		}
		parts = append(parts, TemplatePart[P]{Kind: TemplatePartPlaceholder, Placeholder: placeholder})
		remaining = afterOpen[end+1:]
	}

	if remaining != "" {
		literal, err := literalPart[P](value, remaining)
		if err != nil {
			return Template[P]{}, err
		}
		parts = append(parts, literal)
	}
	return Template[P]{parts: parts}, nil
}

func literalPart[P Placeholder](source, literal string) (TemplatePart[P], error) {
	if strings.ContainsRune(literal, '}') {
		return TemplatePart[P]{}, TemplateError{Kind: TemplateErrorUnmatchedClosingBrace, Detail: source}
	}
	return TemplatePart[P]{Kind: TemplatePartLiteral, Literal: literal}, nil
}

func matchPlaceholder[P Placeholder](token string, placeholders []P) (P, error) {
	var zero P
	if token == "" {
		return zero, TemplateError{Kind: TemplateErrorEmptyPlaceholder}
	}
	for _, placeholder := range placeholders {
		if placeholder.Token() == token {
			return placeholder, nil
		}
	}
	return zero, TemplateError{Kind: TemplateErrorUnknownPlaceholder, Detail: token}
}

// Parts returns the parsed template parts in source order.
func (t Template[P]) Parts() []TemplatePart[P] { return t.parts }

// Contains reports whether the template references placeholder.
func (t Template[P]) Contains(placeholder P) bool {
	for _, part := range t.parts {
		if part.Kind == TemplatePartPlaceholder && part.Placeholder == placeholder {
			return true
		}
	}
	return false
}

// RenderWith renders the template, resolving each placeholder through render. A
// render error is wrapped as a TemplateError of kind TemplateErrorRender.
func (t Template[P]) RenderWith(render func(P) (string, error)) (string, error) {
	var builder strings.Builder
	for _, part := range t.parts {
		switch part.Kind {
		case TemplatePartLiteral:
			builder.WriteString(part.Literal)
		case TemplatePartPlaceholder:
			value, err := render(part.Placeholder)
			if err != nil {
				return "", TemplateError{Kind: TemplateErrorRender, Detail: err.Error()}
			}
			builder.WriteString(value)
		}
	}
	return builder.String(), nil
}
