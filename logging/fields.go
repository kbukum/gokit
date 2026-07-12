package logging

import (
	"time"
)

// Standard field key constants for structured logging.
const (
	FieldComponent     = "component"
	FieldTraceID       = "trace_id"
	FieldSpanID        = "span_id"
	FieldRequestID     = "request_id"
	FieldCorrelationID = "correlation_id"
	FieldUserID        = "user_id"
	FieldSessionID     = "session_id"
	FieldOperation     = "operation"
	FieldStatus        = "status"
	FieldError         = "error"
	FieldDuration      = "duration_ms"
	FieldPlatform      = "platform"
	FieldPhase         = "phase"
	FieldContainerID   = "container_id"
	FieldEmail         = "email"

	// Unified schema fields — consistent across gokit, rskit, and pykit.
	FieldService     = "service"
	FieldEnvironment = "environment"
	FieldTimestamp   = "timestamp"
	FieldLevel       = "level"
	FieldMessage     = "message"
	FieldVersion     = "version"
)

// Fields builds a map[string]any from alternating key-value pairs.
//
//	logger.Info("done", logger.Fields("op", "save", "id", 42))
func Fields(kvs ...any) map[string]any {
	m := make(map[string]any, len(kvs)/2)
	for i := 0; i < len(kvs)-1; i += 2 {
		if key, ok := kvs[i].(string); ok {
			m[key] = kvs[i+1]
		}
	}
	return m
}

// ErrorFields creates fields for an operation that failed.
func ErrorFields(op string, err error) map[string]any {
	m := map[string]any{
		FieldOperation: op,
	}
	if err != nil {
		m[FieldError] = err.Error()
	}
	return m
}

// DurationFields creates fields for a timed operation.
func DurationFields(op string, d time.Duration) map[string]any {
	return map[string]any{
		FieldOperation: op,
		FieldDuration:  d.Milliseconds(),
	}
}

// MergeWithError adds an error field to an existing map.
func MergeWithError(fields map[string]any, err error) map[string]any {
	if fields == nil {
		fields = make(map[string]any)
	}
	if err != nil {
		fields[FieldError] = err.Error()
	}
	return fields
}

// MergeWithDuration adds a duration field to an existing map.
func MergeWithDuration(fields map[string]any, d time.Duration) map[string]any {
	if fields == nil {
		fields = make(map[string]any)
	}
	fields[FieldDuration] = d.Milliseconds()
	return fields
}

// ServiceFields creates the standard service identification fields
// for the unified log schema (consistent across gokit, rskit, and pykit).
func ServiceFields(service, environment, version string) map[string]any {
	return map[string]any{
		FieldService:     service,
		FieldEnvironment: environment,
		FieldVersion:     version,
	}
}
