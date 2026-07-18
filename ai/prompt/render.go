package prompt

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/kbukum/gokit/ai/chat"
)

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// identPattern is the placeholder identifier pattern used by validateSyntax.
// Promoted to package level (vs compiled per call) so each prompt render pays no compile cost.
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Render replaces {{var}} placeholders with values from data.
func (pt *Template) Render(data any) (string, error) {
	if pt == nil {
		return "", fmt.Errorf("prompt: template is nil")
	}
	vars, err := variablesFrom(data)
	if err != nil {
		return "", fmt.Errorf("prompt: render template %q: %w", pt.Name, err)
	}
	rendered, err := Render(pt.Template, vars)
	if err != nil {
		return "", fmt.Errorf("prompt: render template %q: %w", pt.Name, err)
	}
	return rendered, nil
}

// RenderToMessage renders the template and returns it as a SystemMessage.
func (pt *Template) RenderToMessage(data any) (chat.Message, error) {
	text, err := pt.Render(data)
	if err != nil {
		return nil, err
	}
	return chat.System(text), nil
}

// Render replaces {{identifier}} placeholders in tmpl using variables.
func Render(tmpl string, variables map[string]any) (string, error) {
	if err := validateSyntax(tmpl); err != nil {
		return "", err
	}
	var missing []string
	rendered := placeholderPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		value, ok := variables[name]
		if !ok || value == nil {
			missing = append(missing, name)
			return match
		}
		return fmt.Sprint(value)
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("prompt: missing template variables: %s", strings.Join(unique(missing), ", "))
	}
	return rendered, nil
}

func validateSyntax(tmpl string) error {
	for i := 0; i < len(tmpl); i++ {
		if !strings.HasPrefix(tmpl[i:], "{{") {
			continue
		}
		end := strings.Index(tmpl[i+2:], "}}")
		if end < 0 {
			return fmt.Errorf("unclosed placeholder")
		}
		body := strings.TrimSpace(tmpl[i+2 : i+2+end])
		if !identPattern.MatchString(body) {
			return fmt.Errorf("invalid placeholder %q", body)
		}
		i += end + 3
	}
	if strings.Contains(tmpl, "}}") && !strings.Contains(tmpl, "{{") {
		return fmt.Errorf("unopened placeholder")
	}
	return nil
}

func variablesFrom(data any) (map[string]any, error) {
	if data == nil {
		return map[string]any{}, nil
	}
	switch v := data.(type) {
	case map[string]any:
		return v, nil
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, value := range v {
			out[key] = value
		}
		return out, nil
	}
	rv := reflect.ValueOf(data)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return map[string]any{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		out := map[string]any{}
		if err := json.Unmarshal(encoded, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("variables must be a map or struct")
}

func unique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
