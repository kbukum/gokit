package authz

import "strings"

// MatchPattern checks if a permission pattern matches a required permission.
// Supports "resource:action" format with wildcards:
//
//   - "*:*"          matches everything
//   - "article:*"    matches "article:read", "article:write", etc.
//   - "*:read"       matches "article:read", "user:read", etc.
//   - "article:read" matches only "article:read"
//
// Both pattern and required use ":" as the separator.
// If either doesn't contain ":", they are compared as plain strings with wildcard support.
func MatchPattern(pattern, required string) bool {
	// Exact match or universal wildcard
	if pattern == required || pattern == "*" || pattern == "*:*" {
		return true
	}

	patParts := strings.SplitN(pattern, ":", 2)
	reqParts := strings.SplitN(required, ":", 2)

	// Both must have the same format
	if len(patParts) != len(reqParts) {
		// Pattern has separator but required doesn't (or vice versa) — plain comparison
		return matchWildcard(pattern, required)
	}

	if len(patParts) == 1 {
		return matchWildcard(pattern, required)
	}

	// resource:action format
	return matchWildcard(patParts[0], reqParts[0]) && matchWildcard(patParts[1], reqParts[1])
}

// MatchAny returns true if any of the patterns match the required permission.
func MatchAny(patterns []string, required string) bool {
	for _, p := range patterns {
		if MatchPattern(p, required) {
			return true
		}
	}
	return false
}

// matchWildcard compares two strings where "*" matches anything.
func matchWildcard(pattern, value string) bool {
	return pattern == "*" || pattern == value
}
