package authz

import (
	"fmt"
	"slices"
)

// Attributes holds request, subject, or resource attributes for ABAC evaluation.
type Attributes map[string]string

// Subject is the caller being authorized.
type Subject struct {
	ID         string
	Roles      []string
	Attributes Attributes
}

// Resource is the target being accessed.
type Resource struct {
	Type       string
	ID         string
	Attributes Attributes
}

// Request is the canonical authorization input.
type Request struct {
	Subject  Subject
	Resource Resource
	Action   string
	Context  Attributes
}

// Permission is an RBAC grant with wildcard-capable resource/action matching.
type Permission struct {
	Resource string
	Action   string
}

// Matches reports whether the permission covers the requested resource/action.
func (p Permission) Matches(resource, action string) bool {
	return MatchPattern(p.Resource, resource) && MatchPattern(p.Action, action)
}

// Role defines an RBAC role and any inherited parent roles.
type Role struct {
	Name        string
	Inherits    []string
	Permissions []Permission
}

// Effect determines the result of a policy match.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// AttributeSource selects where a condition reads a value from.
type AttributeSource string

const (
	AttributeSourceSubject  AttributeSource = "subject"
	AttributeSourceResource AttributeSource = "resource"
	AttributeSourceContext  AttributeSource = "context"
)

// Operator defines the supported ABAC comparison.
type Operator string

const (
	OperatorEquals    Operator = "equals"
	OperatorNotEquals Operator = "not_equals"
	OperatorOneOf     Operator = "one_of"
)

// Condition compares a source attribute against literal values or another attribute.
type Condition struct {
	Source        AttributeSource
	Key           string
	Operator      Operator
	Values        []string
	CompareSource AttributeSource
	CompareKey    string
}

// Policy is an ABAC rule evaluated alongside RBAC grants.
type Policy struct {
	Name       string
	Effect     Effect
	Actions    []string
	Resources  []string
	Conditions []Condition
}

// Decision is the authorization result.
type Decision struct {
	Allowed bool
	Reason  string
}

// Engine evaluates RBAC role grants and ABAC policies with explicit default-deny semantics.
type Engine struct {
	roles    map[string]Role
	policies []Policy
}

// NewEngine constructs an authorization engine. Inputs are deep-copied so callers cannot mutate engine state after construction.
func NewEngine(roles []Role, policies []Policy) (*Engine, error) {
	roleIndex := make(map[string]Role, len(roles))
	for _, role := range roles {
		if role.Name == "" {
			return nil, fmt.Errorf("authz: role name is required")
		}
		if _, exists := roleIndex[role.Name]; exists {
			return nil, fmt.Errorf("authz: duplicate role %q", role.Name)
		}
		roleIndex[role.Name] = Role{
			Name:        role.Name,
			Inherits:    slices.Clone(role.Inherits),
			Permissions: slices.Clone(role.Permissions),
		}
	}
	copiedPolicies := make([]Policy, len(policies))
	for i, p := range policies {
		conditions := make([]Condition, len(p.Conditions))
		for j, condition := range p.Conditions {
			conditions[j] = Condition{
				Source:        condition.Source,
				Key:           condition.Key,
				Operator:      condition.Operator,
				Values:        slices.Clone(condition.Values),
				CompareSource: condition.CompareSource,
				CompareKey:    condition.CompareKey,
			}
		}
		copiedPolicies[i] = Policy{
			Name:       p.Name,
			Effect:     p.Effect,
			Actions:    slices.Clone(p.Actions),
			Resources:  slices.Clone(p.Resources),
			Conditions: conditions,
		}
	}
	return &Engine{
		roles:    roleIndex,
		policies: copiedPolicies,
	}, nil
}

// Authorize evaluates the request. Deny policies override allow decisions.
func (e *Engine) Authorize(req Request) Decision {
	for _, policy := range e.policies {
		if policy.Effect == EffectDeny && policy.matches(req) {
			return Decision{Allowed: false, Reason: "denied by policy: " + policy.Name}
		}
	}

	if e.roleAllows(req, req.Subject.Roles, map[string]struct{}{}) {
		return Decision{Allowed: true, Reason: "allowed by role grant"}
	}

	for _, policy := range e.policies {
		if policy.Effect == EffectAllow && policy.matches(req) {
			return Decision{Allowed: true, Reason: "allowed by policy: " + policy.Name}
		}
	}

	return Decision{Allowed: false, Reason: "default deny"}
}

// Allowed is a convenience helper for boolean-only call sites.
func (e *Engine) Allowed(req Request) bool {
	return e.Authorize(req).Allowed
}

func (e *Engine) roleAllows(req Request, roleNames []string, visited map[string]struct{}) bool {
	for _, roleName := range roleNames {
		if _, seen := visited[roleName]; seen {
			continue
		}
		visited[roleName] = struct{}{}

		role, ok := e.roles[roleName]
		if !ok {
			continue
		}
		for _, permission := range role.Permissions {
			if permission.Matches(req.Resource.Type, req.Action) {
				return true
			}
		}
		if e.roleAllows(req, role.Inherits, visited) {
			return true
		}
	}
	return false
}

func (p Policy) matches(req Request) bool {
	if !matchPolicyDimension(p.Resources, req.Resource.Type) || !matchPolicyDimension(p.Actions, req.Action) {
		return false
	}
	for _, condition := range p.Conditions {
		if !condition.matches(req) {
			return false
		}
	}
	return true
}

func matchPolicyDimension(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return false
	}
	return MatchAny(patterns, value)
}

func (c Condition) matches(req Request) bool {
	actual, ok := attributeValue(req, c.Source, c.Key)
	if !ok {
		return false
	}

	expected := c.Values
	if c.CompareSource != "" {
		other, found := attributeValue(req, c.CompareSource, c.CompareKey)
		if !found {
			return false
		}
		expected = []string{other}
	}

	switch c.Operator {
	case OperatorEquals:
		return len(expected) == 1 && actual == expected[0]
	case OperatorNotEquals:
		return len(expected) == 1 && actual != expected[0]
	case OperatorOneOf:
		for _, value := range expected {
			if actual == value {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func attributeValue(req Request, source AttributeSource, key string) (string, bool) {
	switch source {
	case AttributeSourceSubject:
		if key == "id" {
			return req.Subject.ID, req.Subject.ID != ""
		}
		value, ok := req.Subject.Attributes[key]
		return value, ok
	case AttributeSourceResource:
		switch key {
		case "id":
			return req.Resource.ID, req.Resource.ID != ""
		case "type":
			return req.Resource.Type, req.Resource.Type != ""
		default:
			value, ok := req.Resource.Attributes[key]
			return value, ok
		}
	case AttributeSourceContext:
		value, ok := req.Context[key]
		return value, ok
	default:
		return "", false
	}
}
