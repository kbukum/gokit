package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter transforms a tool Result's content before it is sent to the LLM.
// This allows tools to return structured data while presenting it to the model in a more readable form (markdown tables, summaries, etc.).
type Formatter interface {
	// Format transforms the result content. The tool name is provided for context.
	Format(toolName string, result *Result) (string, error)
}

// FormatterFunc adapts a function to the Formatter interface.
type FormatterFunc func(toolName string, result *Result) (string, error)

func (f FormatterFunc) Format(toolName string, result *Result) (string, error) {
	return f(toolName, result)
}

// MarkdownTableFormatter formats JSON array results as markdown tables.
// Non-array results are returned as-is.
var MarkdownTableFormatter Formatter = FormatterFunc(formatMarkdownTable)

// TruncateFormatter returns a Formatter that truncates content to maxLen characters.
func TruncateFormatter(maxLen int) Formatter {
	return FormatterFunc(func(_ string, result *Result) (string, error) {
		content := result.Text()
		if len(content) <= maxLen {
			return content, nil
		}
		return content[:maxLen] + fmt.Sprintf("\n... (truncated, %d total chars)", len(content)), nil
	})
}

// SummaryHeaderFormatter prepends a one-line summary header to the result.
func SummaryHeaderFormatter() Formatter {
	return FormatterFunc(func(toolName string, result *Result) (string, error) {
		content := result.Text()
		if result.IsError {
			return fmt.Sprintf("[%s ERROR]\n%s", toolName, content), nil
		}
		return fmt.Sprintf("[%s OK]\n%s", toolName, content), nil
	})
}

// ChainFormatters applies formatters in sequence.
func ChainFormatters(formatters ...Formatter) Formatter {
	return FormatterFunc(func(toolName string, result *Result) (string, error) {
		current := result
		for _, f := range formatters {
			formatted, err := f.Format(toolName, current)
			if err != nil {
				return "", err
			}
			current = &Result{
				Content:  formatted,
				Output:   result.Output,
				IsError:  result.IsError,
				Metadata: result.Metadata,
			}
		}
		return current.Content, nil
	})
}

func formatMarkdownTable(_ string, result *Result) (string, error) {
	content := result.Text()
	if content == "" {
		return content, nil
	}

	// Try parsing as JSON array of objects. A parse failure or empty array is not an error condition —
	// it just means the result isn't a tabular JSON payload,
	// so we return the original text content as-is for the caller.
	var rows []map[string]any
	if err := json.Unmarshal([]byte(content), &rows); err != nil || len(rows) == 0 {
		return content, nil //nolint:nilerr // intentional fallback to raw content
	}

	// Collect ordered column names from the first row.
	var cols []string
	// Use Output for stable ordering if available.
	src := result.Output
	if src == nil {
		src = []byte(content)
	}
	// Simple ordered-key extraction using json.Decoder.
	cols = extractKeys(src)
	if len(cols) == 0 {
		return content, nil
	}

	var b strings.Builder
	// Header
	b.WriteString("| ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(c)
	}
	b.WriteString(" |\n")
	// Separator
	b.WriteString("| ")
	for i := range cols {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	// Rows
	for _, row := range rows {
		b.WriteString("| ")
		for i, c := range cols {
			if i > 0 {
				b.WriteString(" | ")
			}
			fmt.Fprintf(&b, "%v", row[c])
		}
		b.WriteString(" |\n")
	}

	return b.String(), nil
}

// extractKeys returns the top-level keys of the first object in a JSON array,
// preserving their order as they appear in the source.
func extractKeys(data []byte) []string {
	// Parse the outer array.
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil || len(arr) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(arr[0])))
	t, err := dec.Token() // opening '{'
	if err != nil {
		return nil
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if key, ok := tok.(string); ok {
			keys = append(keys, key)
			// Skip the value.
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				break
			}
		}
	}
	return keys
}
