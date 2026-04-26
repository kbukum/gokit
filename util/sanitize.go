package util

import (
	"regexp"
	"strings"
	"unicode"
)

// unsafePatternRegex detects common SQL injection and XSS patterns.
var unsafePatternRegex = regexp.MustCompile(`(?i)(--|;|'|"|<script|<\/script|javascript:|on\w+=|union\s+select|drop\s+table|insert\s+into|delete\s+from|update\s+.+\s+set)`)

// SanitizeString trims whitespace and removes control characters from s.
func SanitizeString(s string) string {
	s = strings.TrimSpace(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// SanitizeEnvValue cleans an environment variable value by removing surrounding
// quotes and trimming whitespace.
func SanitizeEnvValue(s string) string {
	s = strings.TrimSpace(s)
	// Strip matching surrounding quotes (single or double).
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

// IsSafeString checks whether s passes basic input validation. It is NOT a security
// boundary — use parameterized queries for SQL and proper encoding for HTML output.
// This only catches the most obvious injection patterns and is intended as an
// additional defense-in-depth signal, not a primary safeguard.
func IsSafeString(s string) bool {
	return !unsafePatternRegex.MatchString(s)
}
