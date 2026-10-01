package errors

// ViolationReason is a stable semantic category, independent of the validator used.
type ViolationReason string

const (
	ViolationRequired      ViolationReason = "REQUIRED"
	ViolationInvalidFormat ViolationReason = "INVALID_FORMAT"
	ViolationOutOfRange    ViolationReason = "OUT_OF_RANGE"
	ViolationInvalidValue  ViolationReason = "INVALID_VALUE"
)

// Violation is a single field-level problem within a failed request. It is the
// one shape every transport and every validator produces, so a form can place a
// problem on the exact control that caused it regardless of where the failure
// originated (edge shape rules, a struct validator, or a business rule in app).
type Violation struct {
	// Field uses protobuf names for RPC and JSON names for HTTP structs, with dotted nesting and indexed repeated fields. RPC clients translate through request descriptors.
	Field string `json:"field"`
	// Reason is a semantic UPPER_SNAKE_CASE category, never a raw validator rule ID.
	Reason ViolationReason `json:"reason"`
	// Message is safe, human-readable text describing the problem.
	Message string `json:"message"`
}
