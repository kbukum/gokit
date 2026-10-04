package auth

import (
	"context"
	"testing"
)

type membershipPolicy struct{ allowed map[string]bool }

func (p membershipPolicy) Authorize(ctx context.Context, caller Principal, resource string, scopes []string) error {
	if !p.allowed[resource] {
		return Failure("MEMBERSHIP_REQUIRED")
	}
	return nil
}

func TestAuthorizationNeverGrantsMembership(t *testing.T) {
	p := Principal{Subject: "u", Kind: User, Credential: Session, Reference: "protected", Restrictions: Restrictions{Mode: Unrestricted}}
	policy := membershipPolicy{allowed: map[string]bool{"one": true, "two": true}}
	for _, resource := range []string{"one", "two"} {
		if err := Authorize(context.Background(), policy, p, resource, "read"); err != nil {
			t.Fatal(err)
		}
	}
	p.Credential = APIKey
	p.Restrictions = Restrictions{Mode: Restricted, Resources: []string{"one", "absent"}, Scopes: []string{"read"}}
	if err := Authorize(context.Background(), policy, p, "one", "read"); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"two", "absent"} {
		if err := Authorize(context.Background(), policy, p, resource, "read"); err == nil {
			t.Fatal("ceiling granted membership or exceeded", resource)
		}
	}
	if err := Authorize(context.Background(), nil, p, "one", "read"); err == nil {
		t.Fatal("missing policy accepted")
	}
}
