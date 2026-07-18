package render

import "fmt"

// OutputFormat is a machine-readable output format for CLI renderers.
type OutputFormat int

const (
	// FormatText is human-readable terminal text (the zero value).
	FormatText OutputFormat = iota
	// FormatJSON is a JSON object or array.
	FormatJSON
	// FormatYAML is a YAML document.
	FormatYAML
)

// String returns the canonical lowercase name of the format.
func (f OutputFormat) String() string {
	switch f {
	case FormatText:
		return "text"
	case FormatJSON:
		return "json"
	case FormatYAML:
		return "yaml"
	default:
		return fmt.Sprintf("OutputFormat(%d)", int(f))
	}
}

// ParseOutputFormat parses a format from its lowercase name (text/json/yaml).
//
// The second return value is false for any other value, so the caller can raise its own typed usage error naming the accepted values.
func ParseOutputFormat(name string) (OutputFormat, bool) {
	switch name {
	case "text":
		return FormatText, true
	case "json":
		return FormatJSON, true
	case "yaml":
		return FormatYAML, true
	default:
		return FormatText, false
	}
}
