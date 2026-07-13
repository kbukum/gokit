package authz

import "testing"

func TestEngine_RoleHierarchyAllowsInheritedPermission(t *testing.T) {
	engine, err := NewEngine([]Role{
		{
			Name:        "viewer",
			Permissions: []Permission{{Resource: "article", Action: "read"}},
		},
		{
			Name:        "editor",
			Inherits:    []string{"viewer"},
			Permissions: []Permission{{Resource: "article", Action: "write"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	req := Request{
		Subject:  Subject{ID: "u1", Roles: []string{"editor"}},
		Resource: Resource{Type: "article"},
		Action:   "read",
	}
	if decision := engine.Authorize(req); !decision.Allowed {
		t.Fatalf("expected inherited read permission, got %+v", decision)
	}
}

func TestEngine_DefaultDeny(t *testing.T) {
	engine, err := NewEngine(nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	req := Request{
		Subject:  Subject{ID: "u1"},
		Resource: Resource{Type: "article"},
		Action:   "delete",
	}
	decision := engine.Authorize(req)
	if decision.Allowed || decision.Reason != "default deny" {
		t.Fatalf("expected explicit default deny, got %+v", decision)
	}
	if engine.Allowed(req) {
		t.Fatal("Allowed helper should also default deny")
	}
}

func TestEngine_DenyPolicyOverridesRoleGrant(t *testing.T) {
	engine, err := NewEngine(
		[]Role{{
			Name:        "editor",
			Permissions: []Permission{{Resource: "article", Action: "write"}},
		}},
		[]Policy{{
			Name:      "freeze-publishing",
			Effect:    EffectDeny,
			Actions:   []string{"write"},
			Resources: []string{"article"},
			Conditions: []Condition{{
				Source:   AttributeSourceContext,
				Key:      "change_window",
				Operator: OperatorEquals,
				Values:   []string{"closed"},
			}},
		}},
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	req := Request{
		Subject:  Subject{ID: "u1", Roles: []string{"editor"}},
		Resource: Resource{Type: "article"},
		Action:   "write",
		Context:  Attributes{"change_window": "closed"},
	}
	if decision := engine.Authorize(req); decision.Allowed {
		t.Fatalf("expected deny override, got %+v", decision)
	}
}

func TestEngine_ABACOwnerMatch(t *testing.T) {
	engine, err := NewEngine(nil, []Policy{{
		Name:      "owner-can-read",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"document"},
		Conditions: []Condition{{
			Source:        AttributeSourceSubject,
			Key:           "tenant_id",
			Operator:      OperatorEquals,
			CompareSource: AttributeSourceResource,
			CompareKey:    "tenant_id",
		}, {
			Source:        AttributeSourceSubject,
			Key:           "id",
			Operator:      OperatorEquals,
			CompareSource: AttributeSourceResource,
			CompareKey:    "owner_id",
		}},
	}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	req := Request{
		Subject: Subject{
			ID:         "user-1",
			Attributes: Attributes{"tenant_id": "tenant-a"},
		},
		Resource: Resource{
			Type:       "document",
			Attributes: Attributes{"tenant_id": "tenant-a", "owner_id": "user-1"},
		},
		Action: "read",
	}
	if decision := engine.Authorize(req); !decision.Allowed {
		t.Fatalf("expected owner-based allow, got %+v", decision)
	}
}

func TestEngine_ABACRejectsPrivilegeEscalationAcrossTenant(t *testing.T) {
	engine, err := NewEngine(nil, []Policy{{
		Name:      "tenant-admin",
		Effect:    EffectAllow,
		Actions:   []string{"write"},
		Resources: []string{"invoice"},
		Conditions: []Condition{{
			Source:        AttributeSourceSubject,
			Key:           "tenant_id",
			Operator:      OperatorEquals,
			CompareSource: AttributeSourceResource,
			CompareKey:    "tenant_id",
		}, {
			Source:   AttributeSourceSubject,
			Key:      "role_level",
			Operator: OperatorOneOf,
			Values:   []string{"admin", "owner"},
		}},
	}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	tests := []struct {
		name    string
		request Request
		allow   bool
	}{
		{
			name: "same tenant admin",
			request: Request{
				Subject: Subject{ID: "u1", Attributes: Attributes{"tenant_id": "a", "role_level": "admin"}},
				Resource: Resource{Type: "invoice", Attributes: Attributes{
					"tenant_id": "a",
				}},
				Action: "write",
			},
			allow: true,
		},
		{
			name: "cross tenant denied",
			request: Request{
				Subject: Subject{ID: "u2", Attributes: Attributes{"tenant_id": "a", "role_level": "admin"}},
				Resource: Resource{Type: "invoice", Attributes: Attributes{
					"tenant_id": "b",
				}},
				Action: "write",
			},
			allow: false,
		},
		{
			name: "insufficient role denied",
			request: Request{
				Subject: Subject{ID: "u3", Attributes: Attributes{"tenant_id": "a", "role_level": "viewer"}},
				Resource: Resource{Type: "invoice", Attributes: Attributes{
					"tenant_id": "a",
				}},
				Action: "write",
			},
			allow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.Authorize(tt.request)
			if decision.Allowed != tt.allow {
				t.Fatalf("Authorize() = %+v, want allow=%v", decision, tt.allow)
			}
		})
	}
}

func TestEngine_NewEngineRejectsDuplicateRole(t *testing.T) {
	_, err := NewEngine([]Role{{Name: "dup"}, {Name: "dup"}}, nil)
	if err == nil {
		t.Fatal("expected duplicate role to fail")
	}
}

func TestEngine_NewEngineDeepCopiesConditionValues(t *testing.T) {
	policies := []Policy{{
		Name:      "role-guard",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"report"},
		Conditions: []Condition{{
			Source:   AttributeSourceContext,
			Key:      "role",
			Operator: OperatorOneOf,
			Values:   []string{"admin"},
		}},
	}}

	engine, err := NewEngine(nil, policies)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	policies[0].Conditions[0].Values[0] = "viewer"

	decision := engine.Authorize(Request{
		Resource: Resource{Type: "report"},
		Action:   "read",
		Context:  Attributes{"role": "admin"},
	})
	if !decision.Allowed {
		t.Fatalf("expected engine copy to remain immutable, got %+v", decision)
	}
}

func TestEngine_ConditionBranches(t *testing.T) {
	engine, err := NewEngine(nil, []Policy{{
		Name:      "resource-id-guard",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"report"},
		Conditions: []Condition{{
			Source:   AttributeSourceResource,
			Key:      "id",
			Operator: OperatorNotEquals,
			Values:   []string{"blocked"},
		}, {
			Source:   AttributeSourceContext,
			Key:      "env",
			Operator: OperatorOneOf,
			Values:   []string{"dev", "stage"},
		}},
	}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	req := Request{
		Resource: Resource{Type: "report", ID: "r-1"},
		Action:   "read",
		Context:  Attributes{"env": "stage"},
	}
	if decision := engine.Authorize(req); !decision.Allowed {
		t.Fatalf("expected branch coverage allow, got %+v", decision)
	}
}

func TestEngine_NewEngineRejectsEmptyRoleName(t *testing.T) {
	if _, err := NewEngine([]Role{{Name: ""}}, nil); err == nil {
		t.Fatal("expected empty role name error")
	}
}

func TestEngine_RoleCycleAndUnknownInherit(t *testing.T) {
	engine, err := NewEngine([]Role{
		{Name: "a", Inherits: []string{"b", "missing"}},
		{Name: "b", Inherits: []string{"a"}, Permissions: []Permission{{Resource: "doc", Action: "read"}}},
	}, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	req := Request{
		Subject:  Subject{Roles: []string{"a"}},
		Resource: Resource{Type: "doc"},
		Action:   "read",
	}
	if !engine.Allowed(req) {
		t.Fatal("expected allow via inherited role despite cycle")
	}
	if engine.Allowed(Request{Subject: Subject{Roles: []string{"unknown"}}, Resource: Resource{Type: "doc"}, Action: "read"}) {
		t.Fatal("unknown role must not grant access")
	}
}

func TestEngine_PolicyEmptyDimensionDenies(t *testing.T) {
	engine, err := NewEngine(nil, []Policy{{
		Name:      "no-resources",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: nil,
	}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine.Allowed(Request{Resource: Resource{Type: "doc"}, Action: "read"}) {
		t.Fatal("policy with empty resources must not match")
	}
}

func TestEngine_ConditionMissingAttributeDenies(t *testing.T) {
	engine, err := NewEngine(nil, []Policy{{
		Name:      "needs-attr",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"doc"},
		Conditions: []Condition{{
			Source:   AttributeSourceSubject,
			Key:      "team",
			Operator: OperatorEquals,
			Values:   []string{"x"},
		}},
	}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine.Allowed(Request{Resource: Resource{Type: "doc"}, Action: "read"}) {
		t.Fatal("missing subject attribute must deny")
	}
}

func TestEngine_ConditionCompareSource(t *testing.T) {
	policy := Policy{
		Name:      "owner-match",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"doc"},
		Conditions: []Condition{{
			Source:        AttributeSourceSubject,
			Key:           "id",
			Operator:      OperatorEquals,
			CompareSource: AttributeSourceResource,
			CompareKey:    "owner",
		}},
	}
	engine, err := NewEngine(nil, []Policy{policy})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	match := Request{
		Subject:  Subject{ID: "u1"},
		Resource: Resource{Type: "doc", Attributes: Attributes{"owner": "u1"}},
		Action:   "read",
	}
	if !engine.Allowed(match) {
		t.Fatal("expected allow when subject id equals resource owner")
	}

	noCompare := Request{
		Subject:  Subject{ID: "u1"},
		Resource: Resource{Type: "doc"},
		Action:   "read",
	}
	if engine.Allowed(noCompare) {
		t.Fatal("missing compare attribute must deny")
	}
}

func TestEngine_AttributeValueSources(t *testing.T) {
	policy := Policy{
		Name:      "resource-type",
		Effect:    EffectAllow,
		Actions:   []string{"read"},
		Resources: []string{"doc"},
		Conditions: []Condition{{
			Source:   AttributeSourceResource,
			Key:      "type",
			Operator: OperatorOneOf,
			Values:   []string{"doc", "file"},
		}},
	}
	engine, err := NewEngine(nil, []Policy{policy})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if !engine.Allowed(Request{Resource: Resource{Type: "doc"}, Action: "read"}) {
		t.Fatal("expected allow via resource type attribute")
	}
}
