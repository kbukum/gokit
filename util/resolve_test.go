package util

import (
	"errors"
	"testing"
)

func TestResolveUniqueMatch(t *testing.T) {
	t.Parallel()
	candidates := []string{"build", "test", "lint"}
	got, ok, err := ResolveUnique("test", candidates, func(s string) string { return s })
	if err != nil || !ok || got != "test" {
		t.Fatalf("ResolveUnique = (%q, %v, %v)", got, ok, err)
	}
}

func TestResolveUniqueNoMatch(t *testing.T) {
	t.Parallel()
	_, ok, err := ResolveUnique("missing", []string{"a", "b"}, func(s string) string { return s })
	if ok || err != nil {
		t.Fatalf("expected (false,nil), got (%v,%v)", ok, err)
	}
}

func TestResolveUniqueAmbiguous(t *testing.T) {
	t.Parallel()
	candidates := []string{"dup", "dup", "other"}
	_, ok, err := ResolveUnique("dup", candidates, func(s string) string { return s })
	if ok {
		t.Fatal("expected not-ok on ambiguity")
	}
	var amb *Ambiguity[string]
	if !errors.As(err, &amb) {
		t.Fatalf("expected *Ambiguity, got %v", err)
	}
	if amb.Input != "dup" || len(amb.Matches) != 2 {
		t.Errorf("ambiguity = %+v", amb)
	}
}
