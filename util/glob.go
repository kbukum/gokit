package util

// GlobMatch reports whether pattern matches text, treating '*' and '?' as wildcards.
//
// '*' matches any run of characters (including none) and '?' matches exactly one
// character; every other character matches itself. Matching is rune-oriented and
// case-sensitive, with no path or separator semantics, so it composes over
// identifiers, names, or topic segments. The two-pointer algorithm uses constant
// backtracking state (worst case O(len(pattern)*len(text)), never exponential).
func GlobMatch(pattern, text string) bool {
	if !HasWildcard(pattern) {
		return pattern == text
	}
	return wildcardMatch([]rune(pattern), []rune(text))
}

// HasWildcard reports whether pattern contains any wildcard metacharacter ('*' or '?').
func HasWildcard(pattern string) bool {
	for _, c := range pattern {
		if c == '*' || c == '?' {
			return true
		}
	}
	return false
}

// Glob is a compiled glob pattern that can be matched against many candidates.
// A wildcard pattern is parsed once; a plain literal keeps no parsed form and
// compares directly. Matching semantics are identical to GlobMatch.
type Glob struct {
	pattern string
	parsed  []rune // nil for a plain literal
}

// NewGlob compiles pattern into a reusable matcher.
func NewGlob(pattern string) Glob {
	g := Glob{pattern: pattern}
	if HasWildcard(pattern) {
		g.parsed = []rune(pattern)
	}
	return g
}

// IsLiteral reports whether the pattern is a plain literal with no wildcards.
func (g Glob) IsLiteral() bool { return g.parsed == nil }

// Pattern returns the source pattern string.
func (g Glob) Pattern() string { return g.pattern }

// Matches reports whether the pattern matches text.
func (g Glob) Matches(text string) bool {
	if g.parsed == nil {
		return g.pattern == text
	}
	return wildcardMatch(g.parsed, []rune(text))
}

// wildcardMatch is a two-pointer matcher supporting '*' (any run) and '?' (one rune).
func wildcardMatch(pattern, text []rune) bool {
	p, t := 0, 0
	star := -1
	mark := 0
	for t < len(text) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == text[t]):
			p++
			t++
		case p < len(pattern) && pattern[p] == '*':
			star = p
			mark = t
			p++
		case star >= 0:
			p = star + 1
			mark++
			t = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
