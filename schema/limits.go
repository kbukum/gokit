package schema

import "fmt"

// ValidationLimits bounds the structural size of untrusted JSON documents (schemas and values) before they are compiled
// or validated, guarding against resource-exhaustion from adversarial input.
type ValidationLimits struct {
	// MaxDepth is the maximum structural nesting depth.
	MaxDepth int
	// MaxNodes is the maximum number of JSON nodes (values) in the document.
	MaxNodes int
	// MaxStringBytes is the maximum UTF-8 byte length of a single string value.
	MaxStringBytes int
	// MaxKeyBytes is the maximum UTF-8 byte length of a single object key.
	MaxKeyBytes int
	// MaxTotalStringBytes is the maximum cumulative UTF-8 byte length across all object keys
	// and string values.
	MaxTotalStringBytes int
}

// DefaultLimits returns the default structural limits applied when none are specified.
// The values mirror the cross-kit defaults.
func DefaultLimits() ValidationLimits {
	return ValidationLimits{
		MaxDepth:            128,
		MaxNodes:            100_000,
		MaxStringBytes:      1 << 20, // 1 MiB
		MaxKeyBytes:         16_384,
		MaxTotalStringBytes: 1 << 24, // 16 MiB
	}
}

// LimitError is returned when a JSON document exceeds a ValidationLimits bound.
type LimitError struct {
	// Subject identifies the document that violated a limit (e.g. "schema").
	Subject string
	// Message describes the violated bound.
	Message string
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s", e.Subject, e.Message)
}

// check enforces the limits against a decoded JSON document (as produced by json.Unmarshal into any),
// returning a *LimitError on the first violation.
func (l ValidationLimits) check(subject string, value any) error {
	type frame struct {
		node  any
		depth int
	}
	stack := []frame{{node: value, depth: 1}}
	nodes := 0
	totalStringBytes := 0

	fail := func(msg string) error { return &LimitError{Subject: subject, Message: msg} }

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		nodes++
		if nodes > l.MaxNodes {
			return fail(fmt.Sprintf("exceeds maximum JSON node count %d", l.MaxNodes))
		}
		if f.depth > l.MaxDepth {
			return fail(fmt.Sprintf("exceeds maximum JSON depth %d", l.MaxDepth))
		}

		switch node := f.node.(type) {
		case map[string]any:
			for key, child := range node {
				if len(key) > l.MaxKeyBytes {
					return fail(fmt.Sprintf("object key exceeds maximum %d bytes", l.MaxKeyBytes))
				}
				totalStringBytes += len(key)
				if totalStringBytes > l.MaxTotalStringBytes {
					return fail(fmt.Sprintf("exceeds maximum cumulative string bytes %d", l.MaxTotalStringBytes))
				}
				stack = append(stack, frame{node: child, depth: f.depth + 1})
			}
		case []any:
			for _, child := range node {
				stack = append(stack, frame{node: child, depth: f.depth + 1})
			}
		case string:
			if len(node) > l.MaxStringBytes {
				return fail(fmt.Sprintf("string value exceeds maximum %d bytes", l.MaxStringBytes))
			}
			totalStringBytes += len(node)
			if totalStringBytes > l.MaxTotalStringBytes {
				return fail(fmt.Sprintf("exceeds maximum cumulative string bytes %d", l.MaxTotalStringBytes))
			}
		}
	}
	return nil
}
