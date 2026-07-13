package query

import (
	"strings"
	"testing"
)

func TestIsSafeIdentifier_Valid(t *testing.T) {
	valid := []string{
		"name",
		"created_at",
		"_private",
		"col1",
		"users.name",
		"public.users.created_at",
		"A",
	}
	for _, id := range valid {
		if !isSafeIdentifier(id) {
			t.Errorf("isSafeIdentifier(%q) = false, want true", id)
		}
	}
}

func TestIsSafeIdentifier_Invalid(t *testing.T) {
	invalid := []string{
		"",
		" name",
		"name ",
		"1col",
		"name; DROP TABLE users",
		"name OR 1=1",
		"name)--",
		"name'",
		"na me",
		"count(*)",
		"users.",
		".name",
		"users..name",
		"name;",
		"a-b",
		"a,b",
		"a=b",
	}
	for _, id := range invalid {
		if isSafeIdentifier(id) {
			t.Errorf("isSafeIdentifier(%q) = true, want false", id)
		}
	}
}

// FuzzIsSafeIdentifier guarantees the whitelist never accepts an identifier that
// carries SQL metacharacters and never panics on arbitrary input.
func FuzzIsSafeIdentifier(f *testing.F) {
	seeds := []string{"name", "users.id", "", "1=1", "a;b", "a'b", "a b", "a(b)"}
	for _, s := range seeds {
		f.Add(s)
	}
	const metacharacters = " '\"`;()[]{}*/\\+-=<>,%!&|#\t\n\r"
	f.Fuzz(func(t *testing.T, s string) {
		if isSafeIdentifier(s) && strings.ContainsAny(s, metacharacters) {
			t.Fatalf("accepted identifier with metacharacters: %q", s)
		}
	})
}
