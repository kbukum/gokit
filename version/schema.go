package version

import (
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
)

// SupportedSchema returns the configured schema version, or supported when
// configured is nil, rejecting any value the current code does not support.
//
// It is a general-purpose gate for versioned documents (config files, manifests,
// on-disk formats) that declare a schema field and must reject any version the
// current code cannot safely interpret. This is distinct from semantic-version
// parsing: a schema version is typically a small monotonic integer, so the gate
// is generic over any comparable value.
//
// A nil configured value defaults to supported. A configured value that differs
// from supported yields a typed invalid-input AppError.
func SupportedSchema[T comparable](field string, configured *T, supported T) (T, error) {
	schema := supported
	if configured != nil {
		schema = *configured
	}
	if schema != supported {
		return schema, apperrors.InvalidInput(field,
			fmt.Sprintf("unsupported schema %v; supported schema is %v", schema, supported))
	}
	return schema, nil
}
