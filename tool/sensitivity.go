package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Decision is the canonical sensitivity-evaluator outcome.
type Decision string

const (
	// DecisionAllow lets the tool call proceed without further checks.
	DecisionAllow Decision = "allow"
	// DecisionDeny blocks the tool call outright.
	DecisionDeny Decision = "deny"
	// DecisionRequireApproval routes the call through HumanApproval before
	// dispatch. If no HumanApproval is configured the call is denied.
	DecisionRequireApproval Decision = "require_approval"
)

// ToolCall is the validated tool invocation passed to evaluators.
type ToolCall struct {
	// Name is the registered tool name.
	Name string
	// ToolUseID is the call identifier (mirrors ai.ToolUseBlock.ID); may be empty.
	ToolUseID string
	// Input is the validated JSON input.
	Input json.RawMessage
	// Definition is the tool's full definition, including envelope.
	Definition Definition
}

// SensitivityEvaluator decides whether a tool call requires deny / approval
// based on the tool's Envelope.SensitiveInvocations predicates. Returning a
// non-nil error halts dispatch with an internal failure (audited as
// "evaluator_error").
type SensitivityEvaluator interface {
	Evaluate(ctx context.Context, call ToolCall, predicate SensitivePredicate) (Decision, string, error)
}

// HumanApproval gates a tool call when the SensitivityEvaluator returns
// DecisionRequireApproval. Implementations elicit operator approval out of
// band (UI, chat, ticketing). Returning false denies the call; returning a
// non-nil error halts dispatch.
type HumanApproval interface {
	Approve(ctx context.Context, call ToolCall) (bool, error)
}

// DenyOnSensitive is the default SensitivityEvaluator: any matching
// SensitivePredicate denies the call. It does not attempt to evaluate the
// predicate body — predicate matching is structural (presence of any predicate
// in Envelope.SensitiveInvocations is enough to deny). Operators can swap in
// a richer evaluator that inspects ToolCall.Input.
type DenyOnSensitive struct{}

// Evaluate denies the call with a stable reason.
func (DenyOnSensitive) Evaluate(_ context.Context, _ ToolCall, p SensitivePredicate) (Decision, string, error) {
	return DecisionDeny, fmt.Sprintf("sensitive invocation denied: %s %s", p.Matcher, p.JSONPath), nil
}

// DenyHumanApproval is the default HumanApproval: every call is denied. This
// fails closed when DecisionRequireApproval is returned but no real human is
// in the loop.
type DenyHumanApproval struct{}

// Approve always returns (false, nil).
func (DenyHumanApproval) Approve(context.Context, ToolCall) (bool, error) { return false, nil }

// ErrToolDenied is returned by Registry.Call when the tool is denied by the
// sensitivity evaluator, the human approval step, or an authorizer.
var ErrToolDenied = errors.New("tool: call denied")

// ErrInvalidToolInput is returned by Registry.Call when the raw input fails
// JSON Schema validation against the tool's InputSchema. Validation runs
// before authorization and invocation so untrusted, model-produced arguments
// fail closed and never reach the tool handler.
var ErrInvalidToolInput = errors.New("tool: invalid input")
