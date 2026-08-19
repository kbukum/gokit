package util

import "testing"

func TestGlobMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pattern, text string
		want          bool
	}{
		{"core", "core", true},
		{"core", "cores", false},
		{"core", "cor", false},
		{"item-*", "item-core", true},
		{"item-*", "item-", true},
		{"*-core", "item-core", true},
		{"*", "anything", true},
		{"*", "", true},
		{"co?e", "core", true},
		{"co?e", "coe", false},
		{"co?e", "coree", false},
		{"caf?", "café", true},
		{"*é", "café", true},
		{"caf?", "café!", false},
		{"service:*", "tenant:api", false},
		{"service:*", "service:api", true},
		{"a**", "a", true},
		{"a*b*", "ab", true},
	}
	for _, c := range cases {
		if got := GlobMatch(c.pattern, c.text); got != c.want {
			t.Errorf("GlobMatch(%q, %q) = %v, want %v", c.pattern, c.text, got, c.want)
		}
	}
}

func TestHasWildcard(t *testing.T) {
	t.Parallel()
	if !HasWildcard("item-*") || !HasWildcard("co?e") || HasWildcard("core") {
		t.Fatal("HasWildcard mismatch")
	}
}

func TestGlobCompiled(t *testing.T) {
	t.Parallel()
	literal := NewGlob("core")
	if !literal.IsLiteral() || !literal.Matches("core") || literal.Matches("cores") {
		t.Fatal("literal glob mismatch")
	}
	pattern := NewGlob("item-*")
	if pattern.IsLiteral() || !pattern.Matches("item-core") || pattern.Pattern() != "item-*" {
		t.Fatal("wildcard glob mismatch")
	}
}
