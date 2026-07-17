// Package schema validates dataset records against a compiled JSON Schema. It
// wraps the canonical [github.com/kbukum/gokit/schema] validator and fails
// closed: any structural or validation error rejects the record.
package schema
