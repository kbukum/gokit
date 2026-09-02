package authz

import (
	"encoding/json"
	"fmt"
)

// DecisionKind enumerates the transport-agnostic authorization outcomes exchanged
// across service boundaries.
type DecisionKind uint8

const (
	// DecisionInvalid is the zero value and never permits a request, so an
	// uninitialized BoundaryDecision fails closed.
	DecisionInvalid DecisionKind = iota
	// DecisionAllow permits the request.
	DecisionAllow
	// DecisionDeny refuses the request with a reason.
	DecisionDeny
	// DecisionRequiresHumanApproval defers the request to a human approver with a reason.
	DecisionRequiresHumanApproval
)

// String implements fmt.Stringer.
func (k DecisionKind) String() string {
	switch k {
	case DecisionAllow:
		return "Allow"
	case DecisionDeny:
		return "Deny"
	case DecisionRequiresHumanApproval:
		return "RequiresHumanApproval"
	default:
		return "Invalid"
	}
}

// BoundaryDecision is a transport-agnostic authorization decision exchanged across
// service boundaries. It serializes as a tagged union: Allow encodes as the string
// "Allow"; Deny and RequiresHumanApproval encode as a single-key object carrying the
// reason, e.g. {"Deny":"not an owner"}.
type BoundaryDecision struct {
	kind   DecisionKind
	reason string
}

// Allow returns an allow decision.
func Allow() BoundaryDecision { return BoundaryDecision{kind: DecisionAllow} }

// Deny returns a deny decision carrying the given reason.
func Deny(reason string) BoundaryDecision {
	return BoundaryDecision{kind: DecisionDeny, reason: reason}
}

// RequiresHumanApproval returns a decision that defers to a human approver with the given reason.
func RequiresHumanApproval(reason string) BoundaryDecision {
	return BoundaryDecision{kind: DecisionRequiresHumanApproval, reason: reason}
}

// Kind reports which outcome this decision carries.
func (d BoundaryDecision) Kind() DecisionKind { return d.kind }

// Allowed reports whether the decision permits the request.
func (d BoundaryDecision) Allowed() bool { return d.kind == DecisionAllow }

// Reason returns the explanation for a Deny or RequiresHumanApproval decision, or "" for Allow.
func (d BoundaryDecision) Reason() string { return d.reason }

// MarshalJSON implements the tagged-union wire shape.
func (d BoundaryDecision) MarshalJSON() ([]byte, error) {
	switch d.kind {
	case DecisionAllow:
		return []byte(`"Allow"`), nil
	case DecisionDeny:
		return json.Marshal(map[string]string{"Deny": d.reason})
	case DecisionRequiresHumanApproval:
		return json.Marshal(map[string]string{"RequiresHumanApproval": d.reason})
	default:
		return nil, fmt.Errorf("authz: invalid boundary decision kind %d", d.kind)
	}
}

// UnmarshalJSON decodes the tagged-union wire shape.
func (d *BoundaryDecision) UnmarshalJSON(data []byte) error {
	var unit string
	if err := json.Unmarshal(data, &unit); err == nil {
		if unit != "Allow" {
			return fmt.Errorf("authz: unknown boundary decision %q", unit)
		}
		*d = Allow()
		return nil
	}

	var obj map[string]string
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("authz: decode boundary decision: %w", err)
	}
	if len(obj) != 1 {
		return fmt.Errorf("authz: boundary decision must carry exactly one variant, got %d", len(obj))
	}
	for variant, reason := range obj {
		switch variant {
		case "Deny":
			*d = Deny(reason)
		case "RequiresHumanApproval":
			*d = RequiresHumanApproval(reason)
		default:
			return fmt.Errorf("authz: unknown boundary decision variant %q", variant)
		}
	}
	return nil
}

// Boundary converts an engine Decision into its transport-agnostic form. An allowed
// decision maps to Allow; otherwise it maps to Deny carrying the decision reason.
func (d Decision) Boundary() BoundaryDecision {
	if d.Allowed {
		return Allow()
	}
	return Deny(d.Reason)
}

// BoundaryRequest is the transport-agnostic authorization request exchanged across
// service boundaries. Attributes is an opaque JSON document (the documented opaque
// exception) carrying structured request context.
type BoundaryRequest struct {
	// Principal identifies the caller being authorized.
	Principal string `json:"principal"`
	// Action is the operation being attempted.
	Action string `json:"action"`
	// Resource identifies the target being accessed.
	Resource string `json:"resource"`
	// Scopes are the scopes relevant to the request. It always serializes as an array.
	Scopes []string `json:"scopes"`
	// Attributes carries additional structured attributes. It serializes as null when absent.
	Attributes json.RawMessage `json:"attributes"`
}

// MarshalJSON normalizes an absent Scopes slice to an empty array for a stable wire shape.
func (r BoundaryRequest) MarshalJSON() ([]byte, error) {
	type wire BoundaryRequest
	w := wire(r)
	if w.Scopes == nil {
		w.Scopes = []string{}
	}
	return json.Marshal(w)
}
