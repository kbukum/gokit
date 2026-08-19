package util

import (
	"strings"
	"unicode/utf8"
)

// Truncate returns a UTF-8-safe prefix of s sized for TruncateEllipsis. When
// truncation is needed the prefix is at most maxBytes-3 bytes, reserving space
// for the ellipsis TruncateEllipsis appends. A rune is never split.
//
//	Truncate("hello world", 8) == "hello"
//	Truncate("hello", 10) == "hello"
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 3 {
		return ""
	}
	idx := maxBytes - 3
	for idx > 0 && !utf8.RuneStart(s[idx]) {
		idx--
	}
	return s[:idx]
}

// TruncateEllipsis truncates s to at most maxBytes bytes, appending "..." when
// truncation occurs and maxBytes > 3. For smaller limits the result is maxBytes
// dots, because there is no room for the full ellipsis.
//
//	TruncateEllipsis("hello world", 8) == "hello..."
func TruncateEllipsis(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	truncated := Truncate(s, maxBytes)
	if truncated == "" {
		if maxBytes < 0 {
			return ""
		}
		return strings.Repeat(".", maxBytes)
	}
	return truncated + "..."
}
