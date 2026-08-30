package logging

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	apperrors "github.com/kbukum/gokit/errors"
)

// OutputType is the logging sink discriminator serialized through the config key "output".
type OutputType string

const (
	// OutputTypeStdout writes log records to standard output.
	OutputTypeStdout OutputType = "stdout"
	// OutputTypeStderr writes log records to standard error.
	OutputTypeStderr OutputType = "stderr"
	// OutputTypeFile appends log records to a file path.
	OutputTypeFile OutputType = "file"
)

// Output is the tagged logging sink configuration. Stdout and stderr marshal as
// bare strings for the ergonomic shorthand; file output marshals as
// {"type":"file","path":"..."} to match the cross-kit tagged enum.
type Output struct {
	Type OutputType `json:"type" yaml:"type" mapstructure:"type"`
	Path string     `json:"path,omitempty" yaml:"path,omitempty" mapstructure:"path"`
}

// OutputStdout returns the stdout logging sink shorthand. It is a constructor
// rather than an exported variable so callers cannot mutate a shared default.
func OutputStdout() Output {
	return Output{Type: OutputTypeStdout}
}

// OutputStderr returns the stderr logging sink shorthand. It is a constructor
// rather than an exported variable so callers cannot mutate a shared default.
func OutputStderr() Output {
	return Output{Type: OutputTypeStderr}
}

// OutputFile returns a file logging sink that appends to path.
func OutputFile(path string) Output {
	return Output{Type: OutputTypeFile, Path: path}
}

// String renders the output as a human-readable label: "stdout"/"stderr" for
// the process streams and "file:<path>" for a file sink. It is a display form;
// file sinks are configured through the tagged object, not this string.
func (o Output) String() string {
	if o.Type == OutputTypeFile {
		return string(OutputTypeFile) + ":" + o.Path
	}
	return string(o.Type)
}

// IsZero reports whether no output was configured.
func (o Output) IsZero() bool {
	return o.Type == "" && o.Path == ""
}

// Validate validates output configuration.
func (o Output) Validate() error {
	switch o.Type {
	case "", OutputTypeStdout, OutputTypeStderr:
		if strings.TrimSpace(o.Path) != "" {
			return apperrors.InvalidInput("logging.output.path", fmt.Sprintf("path is only valid for file output (got type: %s)", o.Type))
		}
		return nil
	case OutputTypeFile:
		if strings.TrimSpace(o.Path) == "" {
			return apperrors.InvalidInput("logging.output.path", "file output path is required")
		}
		return nil
	default:
		return apperrors.InvalidInput("logging.output.type", fmt.Sprintf("logging.output.type must be one of [stdout stderr file] (got: %s)", o.Type))
	}
}

// MarshalJSON serializes stdout/stderr as bare strings and file as a tagged
// object. It fails closed: an invalid Output never produces a wire value. An
// unset (zero) Output normalizes to the effective stdout default so every
// emitted value round-trips back through UnmarshalJSON.
func (o Output) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o.Type == OutputTypeFile {
		return json.Marshal(struct {
			Type OutputType `json:"type"`
			Path string     `json:"path"`
		}{Type: o.Type, Path: o.Path})
	}
	return json.Marshal(o.effectiveStreamType())
}

// UnmarshalJSON accepts stdout/stderr bare strings or a tagged file object.
func (o *Output) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, parseErr := parseOutputString(text)
		if parseErr != nil {
			return parseErr
		}
		*o = parsed
		return nil
	}

	var tagged struct {
		Type OutputType `json:"type"`
		Path string     `json:"path"`
	}
	if err := json.Unmarshal(data, &tagged); err != nil {
		return err
	}
	if tagged.Type == "" {
		return apperrors.InvalidInput("logging.output.type", "logging output object requires a \"type\" discriminator")
	}
	parsed := Output{Type: tagged.Type, Path: tagged.Path}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*o = parsed
	return nil
}

// MarshalYAML serializes stdout/stderr as bare strings and file as a tagged
// object. It fails closed: an invalid Output never produces a wire value. An
// unset (zero) Output normalizes to the effective stdout default so every
// emitted value round-trips back through UnmarshalYAML.
func (o Output) MarshalYAML() (any, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o.Type == OutputTypeFile {
		return map[string]any{"type": string(o.Type), "path": o.Path}, nil
	}
	return string(o.effectiveStreamType()), nil
}

// effectiveStreamType maps an unset stream output to its runtime default,
// stdout, so a zero Output serializes to a value its own decoder accepts.
func (o Output) effectiveStreamType() OutputType {
	if o.Type == "" {
		return OutputTypeStdout
	}
	return o.Type
}

// UnmarshalYAML accepts stdout/stderr bare strings or a tagged file object.
func (o *Output) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err == nil {
		parsed, parseErr := parseOutputString(text)
		if parseErr != nil {
			return parseErr
		}
		*o = parsed
		return nil
	}

	var tagged struct {
		Type OutputType `yaml:"type"`
		Path string     `yaml:"path"`
	}
	if err := unmarshal(&tagged); err != nil {
		return err
	}
	if tagged.Type == "" {
		return apperrors.InvalidInput("logging.output.type", "logging output object requires a \"type\" discriminator")
	}
	parsed := Output{Type: tagged.Type, Path: tagged.Path}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*o = parsed
	return nil
}

// OutputDecodeHook decodes Viper/mapstructure string and tagged-map values into Output.
func OutputDecodeHook() mapstructure.DecodeHookFuncType {
	outputType := reflect.TypeOf(Output{})
	return func(_ reflect.Type, to reflect.Type, data any) (any, error) {
		if to != outputType {
			return data, nil
		}
		return ParseOutput(data)
	}
}

// ParseOutput decodes a config value into Output.
func ParseOutput(value any) (Output, error) {
	switch v := value.(type) {
	case Output:
		return v, v.Validate()
	case OutputType:
		return parseOutputString(string(v))
	case string:
		return parseOutputString(v)
	case map[string]any:
		return parseOutputMap(v)
	case map[any]any:
		normalized := make(map[string]any, len(v))
		for key, val := range v {
			normalized[fmt.Sprint(key)] = val
		}
		return parseOutputMap(normalized)
	default:
		return Output{}, apperrors.InvalidInput("logging.output", fmt.Sprintf("unsupported logging output config type %T", value))
	}
}

func parseOutputString(value string) (Output, error) {
	switch OutputType(strings.ToLower(strings.TrimSpace(value))) {
	case OutputTypeStdout:
		return OutputStdout(), nil
	case OutputTypeStderr:
		return OutputStderr(), nil
	case OutputTypeFile:
		return Output{}, apperrors.InvalidInput("logging.output", "file output requires a tagged object with path")
	default:
		return Output{}, apperrors.InvalidInput("logging.output.type", fmt.Sprintf("logging.output.type must be one of [stdout stderr file] (got: %s)", value))
	}
}

func parseOutputMap(value map[string]any) (Output, error) {
	rawType, ok := value["type"]
	if !ok {
		return Output{}, apperrors.InvalidInput("logging.output.type", "logging output object requires a \"type\" discriminator")
	}
	outputType, ok := rawType.(string)
	if !ok {
		return Output{}, apperrors.InvalidInput("logging.output.type", fmt.Sprintf("logging.output.type must be a string (got: %T)", rawType))
	}
	path := ""
	if rawPath, hasPath := value["path"]; hasPath {
		path, ok = rawPath.(string)
		if !ok {
			return Output{}, apperrors.InvalidInput("logging.output.path", fmt.Sprintf("logging.output.path must be a string (got: %T)", rawPath))
		}
	}
	output := Output{Type: OutputType(strings.ToLower(strings.TrimSpace(outputType))), Path: path}
	if err := output.Validate(); err != nil {
		return Output{}, err
	}
	return output, nil
}
