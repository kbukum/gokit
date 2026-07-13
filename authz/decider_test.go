package authz

import (
	"context"
	"testing"
)

func TestDeciderFunc_Decide(t *testing.T) {
	want := Decision{Allowed: true, Reason: "func"}
	var d Decider = DeciderFunc(func(context.Context, Request) (Decision, error) {
		return want, nil
	})
	got, err := d.Decide(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got != want {
		t.Fatalf("Decide = %+v, want %+v", got, want)
	}
}

func TestEngine_Decide(t *testing.T) {
	engine, err := NewEngine([]Role{{
		Name:        "reader",
		Permissions: []Permission{{Resource: "doc", Action: "read"}},
	}}, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	var d Decider = engine
	dec, err := d.Decide(context.Background(), Request{
		Subject:  Subject{Roles: []string{"reader"}},
		Resource: Resource{Type: "doc"},
		Action:   "read",
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected allow, got %+v", dec)
	}
}
