package errors

// Failure is the transport-neutral JSON failure vocabulary. RetryAfter is a minimum delay in seconds, not a Go duration or permission to repeat an operation.
type Failure struct {
	Code       ErrorCode   `json:"code"`
	Reason     string      `json:"reason,omitempty"`
	Message    string      `json:"message"`
	Violations []Violation `json:"violations,omitempty"`
	Retryable  bool        `json:"retryable"`
	RetryAfter float64     `json:"retryAfter,omitempty"`
	TraceID    string      `json:"traceId,omitempty"`
}

// ToFailure projects approved public fields without diagnostic causes or opaque extensions.
func (e *AppError) ToFailure() Failure {
	f := Failure{
		Code: e.Code, Reason: e.Reason, Message: e.Message,
		Violations: e.Violations, Retryable: e.Retryable, TraceID: e.TraceID,
	}
	if e.Retryable && e.RetryAfter > 0 {
		f.RetryAfter = e.RetryAfter.Seconds()
	}
	return f
}
