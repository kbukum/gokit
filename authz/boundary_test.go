package authz

import (
	"encoding/json"
	"testing"
)

func TestBoundaryDecisionMarshalJSON(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		decision BoundaryDecision
		want     string
	}{
		"allow":                   {Allow(), `"Allow"`},
		"deny":                    {Deny("not an owner"), `{"Deny":"not an owner"}`},
		"requires_human_approval": {RequiresHumanApproval("destructive action"), `{"RequiresHumanApproval":"destructive action"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tc.decision)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := string(data); got != tc.want {
				t.Fatalf("JSON = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBoundaryDecisionUnmarshalJSON(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		wire       string
		wantKind   DecisionKind
		wantReason string
	}{
		"allow":                   {`"Allow"`, DecisionAllow, ""},
		"deny":                    {`{"Deny":"nope"}`, DecisionDeny, "nope"},
		"requires_human_approval": {`{"RequiresHumanApproval":"review"}`, DecisionRequiresHumanApproval, "review"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var d BoundaryDecision
			if err := json.Unmarshal([]byte(tc.wire), &d); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if d.Kind() != tc.wantKind {
				t.Errorf("Kind = %v, want %v", d.Kind(), tc.wantKind)
			}
			if d.Reason() != tc.wantReason {
				t.Errorf("Reason = %q, want %q", d.Reason(), tc.wantReason)
			}
		})
	}
}

func TestBoundaryDecisionUnmarshalRejectsUnknown(t *testing.T) {
	t.Parallel()

	for _, wire := range []string{`"Nope"`, `{"Deny":"a","Extra":"b"}`, `{"Bogus":"x"}`, `{}`, `42`} {
		var d BoundaryDecision
		if err := json.Unmarshal([]byte(wire), &d); err == nil {
			t.Errorf("expected error decoding %s", wire)
		}
	}
}

func TestDecisionToBoundary(t *testing.T) {
	t.Parallel()

	if got := (Decision{Allowed: true, Reason: "ok"}).Boundary(); got.Kind() != DecisionAllow {
		t.Errorf("allowed decision -> %v, want DecisionAllow", got.Kind())
	}
	denied := (Decision{Allowed: false, Reason: "default deny"}).Boundary()
	if denied.Kind() != DecisionDeny || denied.Reason() != "default deny" {
		t.Errorf("denied decision -> %v/%q, want DecisionDeny/default deny", denied.Kind(), denied.Reason())
	}
}

func TestBoundaryDecisionZeroValueFailsClosed(t *testing.T) {
	t.Parallel()

	var d BoundaryDecision
	if d.Kind() != DecisionInvalid {
		t.Errorf("zero-value Kind = %v, want DecisionInvalid", d.Kind())
	}
	if d.Allowed() {
		t.Error("zero-value BoundaryDecision must not be Allowed")
	}
	if _, err := json.Marshal(d); err == nil {
		t.Error("marshaling an invalid BoundaryDecision must error, not emit a permissive shape")
	}
}

func TestBoundaryRequestJSONShape(t *testing.T) {
	t.Parallel()

	req := BoundaryRequest{
		Principal:  "user:1",
		Action:     ActionToolInvoke,
		Resource:   "tool:deploy",
		Scopes:     []string{"read", "write"},
		Attributes: json.RawMessage(`{"env":"prod"}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"principal":"user:1","action":"tool:invoke","resource":"tool:deploy","scopes":["read","write"],"attributes":{"env":"prod"}}`
	if got := string(data); got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestBoundaryRequestEmptyEncodesStableShape(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(BoundaryRequest{Principal: "p", Action: "a", Resource: "r"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"principal":"p","action":"a","resource":"r","scopes":[],"attributes":null}`
	if got := string(data); got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
