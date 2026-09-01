package status

import "fmt"

// Status is the canonical status vocabulary for DAG node execution.
type Status string

// Node execution status values.
const (
	Completed      Status = "completed"
	Failed         Status = "failed"
	Skipped        Status = "skipped"
	Unavailable    Status = "unavailable"
	DepUnavailable Status = "skipped:dep_unavailable"
	DepFailed      Status = "skipped:dep_failed"
	DepSkipped     Status = "skipped:dep_not_ready"
)

// String returns the wire representation of the status.
func (s Status) String() string {
	return string(s)
}

// IsValid reports whether s is one of the known status values.
func (s Status) IsValid() bool {
	switch s {
	case Completed, Failed, Skipped, Unavailable, DepUnavailable, DepFailed, DepSkipped:
		return true
	default:
		return false
	}
}

// Parse converts a wire status string to a Status, rejecting unknown values so a decoded
// graph result cannot carry a status outside the canonical vocabulary.
func Parse(s string) (Status, error) {
	st := Status(s)
	if !st.IsValid() {
		return "", fmt.Errorf("status: unknown status %q", s)
	}
	return st, nil
}

// IsTerminal returns true if the node ran to a terminal outcome (Completed or Failed).
func (s Status) IsTerminal() bool {
	return s == Completed || s == Failed
}

// IsSkipped returns true if the node was skipped for any reason.
func (s Status) IsSkipped() bool {
	return s == Skipped || s == DepUnavailable || s == DepFailed || s == DepSkipped
}

// IsSuccess returns true if the node completed successfully.
func (s Status) IsSuccess() bool {
	return s == Completed
}
