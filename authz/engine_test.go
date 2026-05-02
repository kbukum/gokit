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
