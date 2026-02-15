package logger

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
)

// Fields builds a map[string]interface{} from alternating key-value pairs.
//
//	logger.Info("done", logger.Fields("op", "save", "id", 42))
func Fields(kvs ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(kvs)/2)
	for i := 0; i < len(kvs)-1; i += 2 {
		if key, ok := kvs[i].(string); ok {
			m[key] = kvs[i+1]
		}
	}
	return m
}

// ErrorFields creates fields for an operation that failed.
func ErrorFields(op string, err error) map[string]interface{} {
	return map[string]interface{}{
		FieldOperation: op,
		FieldError:     err.Error(),
	}
}

// DurationFields creates fields for a timed operation.
func DurationFields(op string, d time.Duration) map[string]interface{} {
	return map[string]interface{}{
		FieldOperation: op,
		FieldDuration:  d.Milliseconds(),
	}
}

// MergeWithError adds an error field to an existing map.
func MergeWithError(fields map[string]interface{}, err error) map[string]interface{} {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields[FieldError] = err.Error()
	return fields
}

// MergeWithDuration adds a duration field to an existing map.
func MergeWithDuration(fields map[string]interface{}, d time.Duration) map[string]interface{} {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields[FieldDuration] = d.Milliseconds()
	return fields
}
