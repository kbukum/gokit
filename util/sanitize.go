package util

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// unsafePatternRegex detects common SQL injection and XSS patterns.
	unsafePatternRegex = regexp.MustCompile(`(?i)(--|;|'|"|<script|<\/script|javascript:|on\w+=|union\s+select|drop\s+table|insert\s+into|delete\s+from|update\s+.+\s+set)`)
)

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

// IsSafeString returns false if s contains patterns commonly associated with
// SQL injection or XSS attacks.
func IsSafeString(s string) bool {
	return !unsafePatternRegex.MatchString(s)
}
