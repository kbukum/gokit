package authz

import (
	"sync"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════════════
// MatchPattern
// ═══════════════════════════════════════════════════════════════════════════════

func TestMatchPattern_ExactMatch(t *testing.T) {
	if !MatchPattern("article:read", "article:read") {
		t.Fatal("exact match should return true")
	}
}

func TestMatchPattern_ExactNoMatch(t *testing.T) {
	if MatchPattern("article:read", "article:write") {
		t.Fatal("different action should not match")
	}
}

func TestMatchPattern_UniversalWildcardStarStar(t *testing.T) {
	cases := []string{"article:read", "user:delete", "a:b", ":"}
	for _, req := range cases {
		if !MatchPattern("*:*", req) {
			t.Fatalf("*:* should match %q", req)
		}
	}
}

func TestMatchPattern_UniversalWildcardStar(t *testing.T) {
	cases := []string{"article:read", "anything", "*:*", ""}
	for _, req := range cases {
		if !MatchPattern("*", req) {
			t.Fatalf("* should match %q", req)
		}
	}
}

func TestMatchPattern_ResourceWildcard(t *testing.T) {
	if !MatchPattern("article:*", "article:read") {
		t.Fatal("article:* should match article:read")
	}
	if !MatchPattern("article:*", "article:write") {
		t.Fatal("article:* should match article:write")
	}
	if !MatchPattern("article:*", "article:delete") {
		t.Fatal("article:* should match article:delete")
	}
}

func TestMatchPattern_ResourceWildcardNoMatch(t *testing.T) {
	if MatchPattern("article:*", "user:read") {
		t.Fatal("article:* should NOT match user:read")
	}
}

func TestMatchPattern_ActionWildcard(t *testing.T) {
	if !MatchPattern("*:read", "article:read") {
		t.Fatal("*:read should match article:read")
	}
	if !MatchPattern("*:read", "user:read") {
		t.Fatal("*:read should match user:read")
	}
}

func TestMatchPattern_ActionWildcardNoMatch(t *testing.T) {
	if MatchPattern("*:read", "article:write") {
		t.Fatal("*:read should NOT match article:write")
	}
}

func TestMatchPattern_PlainStringExact(t *testing.T) {
	if !MatchPattern("admin", "admin") {
		t.Fatal("plain string exact match should return true")
	}
}

func TestMatchPattern_PlainStringNoMatch(t *testing.T) {
	if MatchPattern("admin", "editor") {
		t.Fatal("different plain strings should not match")
	}
}

func TestMatchPattern_PlainWildcard(t *testing.T) {
	if !MatchPattern("*", "anything") {
		t.Fatal("* should match any plain string")
	}
}

func TestMatchPattern_FormatMismatch(t *testing.T) {
	// pattern has colon, required does not
	if MatchPattern("article:read", "admin") {
		t.Fatal("colon pattern should not match plain string")
	}
	// pattern is plain, required has colon
	if MatchPattern("admin", "article:read") {
		t.Fatal("plain pattern should not match colon string")
	}
}

func TestMatchPattern_EmptyStrings(t *testing.T) {
	// Two empty strings: exact match
	if !MatchPattern("", "") {
		t.Fatal("empty pattern vs empty required should match (exact)")
	}
}

func TestMatchPattern_EmptyPatternNonEmpty(t *testing.T) {
	if MatchPattern("", "article:read") {
		t.Fatal("empty pattern should not match non-empty required")
	}
}

func TestMatchPattern_NonEmptyPatternEmpty(t *testing.T) {
	if MatchPattern("article:read", "") {
		t.Fatal("non-empty pattern should not match empty required")
	}
}

func TestMatchPattern_SingleColon(t *testing.T) {
	// ":" splits into ["", ""] for both
	if !MatchPattern(":", ":") {
		t.Fatal(": should match :")
	}
}

func TestMatchPattern_MultipleColons(t *testing.T) {
	// SplitN with limit 2: "a:b:c" → ["a", "b:c"]
	if !MatchPattern("a:b:c", "a:b:c") {
		t.Fatal("a:b:c should match a:b:c (exact)")
	}
	if MatchPattern("a:b", "a:b:c") {
		t.Fatal("a:b should not match a:b:c")
	}
}

func TestMatchPattern_NoColonWildcard(t *testing.T) {
	if !MatchPattern("*", "admin") {
		t.Fatal("* should match plain string admin")
	}
}

func TestMatchPattern_CaseSensitivity(t *testing.T) {
	if MatchPattern("Article:Read", "article:read") {
		t.Fatal("matching should be case-sensitive")
	}
	if MatchPattern("ADMIN", "admin") {
		t.Fatal("plain string matching should be case-sensitive")
	}
}

func TestMatchPattern_UnicodePatterns(t *testing.T) {
	if !MatchPattern("文章:読む", "文章:読む") {
		t.Fatal("unicode exact match should work")
	}
	if !MatchPattern("文章:*", "文章:読む") {
		t.Fatal("unicode resource wildcard should work")
	}
	if !MatchPattern("*:読む", "文章:読む") {
		t.Fatal("unicode action wildcard should work")
	}
}

func TestMatchPattern_SpecialCharacters(t *testing.T) {
	if !MatchPattern("user@domain:read", "user@domain:read") {
		t.Fatal("special characters should match exactly")
	}
	if !MatchPattern("path/to/resource:*", "path/to/resource:write") {
		t.Fatal("special chars in resource with wildcard action should work")
	}
}

func TestMatchPattern_VeryLongStrings(t *testing.T) {
	long := make([]byte, 10000)
	for i := range long {
		long[i] = 'a'
	}
	s := string(long)
	if !MatchPattern(s+":read", s+":read") {
		t.Fatal("very long strings should match exactly")
	}
	if !MatchPattern(s+":*", s+":write") {
		t.Fatal("very long resource with wildcard action should work")
	}
}

func TestMatchPattern_WhitespaceInPatterns(t *testing.T) {
	// Leading/trailing whitespace is significant (no trimming)
	if MatchPattern(" article:read", "article:read") {
		t.Fatal("leading whitespace should make patterns differ")
	}
	if MatchPattern("article:read ", "article:read") {
		t.Fatal("trailing whitespace should make patterns differ")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MatchAny
// ═══════════════════════════════════════════════════════════════════════════════

func TestMatchAny_FirstPatternMatches(t *testing.T) {
	if !MatchAny([]string{"article:read", "article:write"}, "article:read") {
		t.Fatal("should match first pattern")
	}
}

func TestMatchAny_LastPatternMatches(t *testing.T) {
	if !MatchAny([]string{"user:read", "user:write", "article:read"}, "article:read") {
		t.Fatal("should match last pattern")
	}
}

func TestMatchAny_NoPatternMatches(t *testing.T) {
	if MatchAny([]string{"user:read", "user:write"}, "article:delete") {
		t.Fatal("should not match when no pattern matches")
	}
}

func TestMatchAny_EmptyPatternList(t *testing.T) {
	if MatchAny([]string{}, "article:read") {
		t.Fatal("empty list should never match")
	}
}

func TestMatchAny_NilPatternSlice(t *testing.T) {
	if MatchAny(nil, "article:read") {
		t.Fatal("nil slice should never match")
	}
}

func TestMatchAny_SinglePatternMatches(t *testing.T) {
	if !MatchAny([]string{"*:*"}, "article:read") {
		t.Fatal("single wildcard pattern should match")
	}
}

func TestMatchAny_SinglePatternNoMatch(t *testing.T) {
	if MatchAny([]string{"user:read"}, "article:write") {
		t.Fatal("single non-matching pattern should return false")
	}
}

func TestMatchAny_DuplicatePatterns(t *testing.T) {
	if !MatchAny([]string{"article:read", "article:read"}, "article:read") {
		t.Fatal("duplicate patterns should still match")
	}
}

func TestMatchAny_AllPatternsMatch(t *testing.T) {
	if !MatchAny([]string{"*:*", "*", "article:read"}, "article:read") {
		t.Fatal("should return true when all patterns match")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MapChecker
// ═══════════════════════════════════════════════════════════════════════════════

func newTestChecker() *MapChecker {
	return NewMapChecker(map[string][]string{
		"admin":  {"*:*"},
		"editor": {"article:read", "article:write", "media:read"},
		"viewer": {"*:read"},
	})
}

func TestMapChecker_NewMapChecker(t *testing.T) {
	checker := newTestChecker()
	if checker == nil {
		t.Fatal("NewMapChecker should not return nil")
	}
}

func TestMapChecker_AdminWildcard(t *testing.T) {
	checker := newTestChecker()
	if !checker.HasPermission("admin", "article:delete") {
		t.Fatal("admin with *:* should allow article:delete")
	}
	if !checker.HasPermission("admin", "user:write") {
		t.Fatal("admin with *:* should allow user:write")
	}
	if !checker.HasPermission("admin", "anything:here") {
		t.Fatal("admin with *:* should allow anything")
	}
}

func TestMapChecker_EditorAllowed(t *testing.T) {
	checker := newTestChecker()
	if !checker.HasPermission("editor", "article:read") {
		t.Fatal("editor should have article:read")
	}
	if !checker.HasPermission("editor", "article:write") {
		t.Fatal("editor should have article:write")
	}
	if !checker.HasPermission("editor", "media:read") {
		t.Fatal("editor should have media:read")
	}
}

func TestMapChecker_EditorDenied(t *testing.T) {
	checker := newTestChecker()
	if checker.HasPermission("editor", "user:delete") {
		t.Fatal("editor should NOT have user:delete")
	}
	if checker.HasPermission("editor", "article:delete") {
		t.Fatal("editor should NOT have article:delete")
	}
}

func TestMapChecker_ViewerActionWildcard(t *testing.T) {
	checker := newTestChecker()
	if !checker.HasPermission("viewer", "article:read") {
		t.Fatal("viewer with *:read should allow article:read")
	}
	if !checker.HasPermission("viewer", "user:read") {
		t.Fatal("viewer with *:read should allow user:read")
	}
	if checker.HasPermission("viewer", "article:write") {
		t.Fatal("viewer with *:read should NOT allow article:write")
	}
}

func TestMapChecker_UnknownSubjectDenied(t *testing.T) {
	checker := newTestChecker()
	if checker.HasPermission("ghost", "article:read") {
		t.Fatal("unknown subject should be denied")
	}
}

func TestMapChecker_KnownSubjectWrongPermission(t *testing.T) {
	checker := newTestChecker()
	if checker.HasPermission("editor", "system:shutdown") {
		t.Fatal("known subject without matching permission should be denied")
	}
}

func TestMapChecker_EmptyPermissionsMap(t *testing.T) {
	checker := NewMapChecker(map[string][]string{})
	if checker.HasPermission("admin", "article:read") {
		t.Fatal("empty map should deny all")
	}
}

func TestMapChecker_NilPermissionsMap(t *testing.T) {
	checker := NewMapChecker(nil)
	if checker.HasPermission("admin", "article:read") {
		t.Fatal("nil map should deny all")
	}
}

func TestMapChecker_SubjectWithEmptyPermissions(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"locked": {},
	})
	if checker.HasPermission("locked", "article:read") {
		t.Fatal("subject with empty permission list should be denied")
	}
}

func TestMapChecker_SpecialCharacters(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"user@domain.com": {"org/repo:push"},
	})
	if !checker.HasPermission("user@domain.com", "org/repo:push") {
		t.Fatal("special characters in subject/permission should work")
	}
}

func TestMapChecker_OverrideSubjectPermissions(t *testing.T) {
	// When a map is passed, last key wins (Go map semantics)
	perms := map[string][]string{
		"editor": {"article:read"},
	}
	checker := NewMapChecker(perms)
	if !checker.HasPermission("editor", "article:read") {
		t.Fatal("editor should have article:read")
	}
	if checker.HasPermission("editor", "article:write") {
		t.Fatal("editor should NOT have article:write")
	}
}

func TestMapChecker_ConcurrentAccess(t *testing.T) {
	checker := newTestChecker()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Concurrent reads should be safe
			checker.HasPermission("admin", "article:read")
			checker.HasPermission("editor", "article:write")
			checker.HasPermission("viewer", "user:read")
			checker.HasPermission("ghost", "anything:here")
		}()
	}
	wg.Wait()
}

func TestMapChecker_LargePermissionSet(t *testing.T) {
	perms := make(map[string][]string)
	for i := 0; i < 1000; i++ {
		subject := "user" + string(rune('0'+i%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i/100%10))
		perms[subject] = []string{"resource:read", "resource:write"}
	}
	checker := NewMapChecker(perms)
	// Spot check
	if !checker.HasPermission("user000", "resource:read") {
		t.Fatal("large set: user000 should have resource:read")
	}
	if checker.HasPermission("nonexistent", "resource:read") {
		t.Fatal("large set: nonexistent user should be denied")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CheckerFunc
// ═══════════════════════════════════════════════════════════════════════════════

func TestCheckerFunc_ReturnsTrue(t *testing.T) {
	fn := CheckerFunc(func(subject, permission string) bool {
		return true
	})
	if !fn.HasPermission("any", "thing") {
		t.Fatal("always-true CheckerFunc should return true")
	}
}

func TestCheckerFunc_ReturnsFalse(t *testing.T) {
	fn := CheckerFunc(func(subject, permission string) bool {
		return false
	})
	if fn.HasPermission("any", "thing") {
		t.Fatal("always-false CheckerFunc should return false")
	}
}

func TestCheckerFunc_ReceivesCorrectArgs(t *testing.T) {
	var gotSubject, gotPermission string
	fn := CheckerFunc(func(subject, permission string) bool {
		gotSubject = subject
		gotPermission = permission
		return true
	})
	fn.HasPermission("alice", "article:read")
	if gotSubject != "alice" {
		t.Fatalf("expected subject alice, got %s", gotSubject)
	}
	if gotPermission != "article:read" {
		t.Fatalf("expected permission article:read, got %s", gotPermission)
	}
}

func TestCheckerFunc_ConcurrentCalls(t *testing.T) {
	fn := CheckerFunc(func(subject, permission string) bool {
		return subject == "admin"
	})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn.HasPermission("admin", "any:thing")
			fn.HasPermission("guest", "any:thing")
		}()
	}
	wg.Wait()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Checker interface compliance
// ═══════════════════════════════════════════════════════════════════════════════

func TestChecker_MapCheckerImplements(t *testing.T) {
	var _ Checker = NewMapChecker(nil)
}

func TestChecker_CheckerFuncImplements(t *testing.T) {
	var _ Checker = CheckerFunc(func(s, p string) bool { return false })
}

// ═══════════════════════════════════════════════════════════════════════════════
// Security-focused tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestSecurity_DefaultDeny_EmptyChecker(t *testing.T) {
	checker := NewMapChecker(map[string][]string{})
	if checker.HasPermission("admin", "article:read") {
		t.Fatal("SECURITY: empty checker must deny by default")
	}
}

func TestSecurity_DefaultDeny_UnknownSubject(t *testing.T) {
	checker := newTestChecker()
	if checker.HasPermission("unknown", "article:read") {
		t.Fatal("SECURITY: unknown subject must be denied")
	}
}

func TestSecurity_WildcardDoesNotEscalateOtherSubjects(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"admin":  {"*:*"},
		"viewer": {"*:read"},
	})
	// viewer should NOT get write through admin's wildcard
	if checker.HasPermission("viewer", "article:write") {
		t.Fatal("SECURITY: viewer must not inherit admin permissions")
	}
}

func TestSecurity_CaseSensitiveSubject(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"admin": {"*:*"},
	})
	if checker.HasPermission("Admin", "article:read") {
		t.Fatal("SECURITY: subject lookup must be case-sensitive")
	}
	if checker.HasPermission("ADMIN", "article:read") {
		t.Fatal("SECURITY: subject lookup must be case-sensitive")
	}
}

func TestSecurity_CaseSensitivePermission(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"editor": {"article:read"},
	})
	if checker.HasPermission("editor", "Article:Read") {
		t.Fatal("SECURITY: permission matching must be case-sensitive")
	}
}

func TestSecurity_EmptySubject(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"admin": {"*:*"},
	})
	if checker.HasPermission("", "article:read") {
		t.Fatal("SECURITY: empty subject must be denied")
	}
}

func TestSecurity_EmptyPermission(t *testing.T) {
	checker := NewMapChecker(map[string][]string{
		"admin": {"*:*"},
	})
	// *:* matches everything including empty via MatchPattern
	// This tests that the behavior is at least defined
	result := checker.HasPermission("admin", "")
	_ = result // behavior is defined by MatchPattern; just ensure no panic
}

func TestSecurity_WildcardOnlyInPatternNotValue(t *testing.T) {
	// If a user sends "*:*" as a permission request, it should NOT auto-grant
	checker := NewMapChecker(map[string][]string{
		"viewer": {"article:read"},
	})
	if checker.HasPermission("viewer", "*:*") {
		t.Fatal("SECURITY: wildcard in requested permission must NOT auto-grant")
	}
}
