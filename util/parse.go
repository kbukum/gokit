package util

import (
	"fmt"
	"strings"
)

// ParseSize parses a human-readable size string (e.g. "10MB", "512KB", "2GB")
// into bytes. Returns defaultBytes if the string cannot be parsed.
func ParseSize(s string, defaultBytes int64) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return defaultBytes
	}

	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = s[:len(s)-2]
	}

	var val int64
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return val * multiplier
	}
	return defaultBytes
}

// MaskSecret hides sensitive parts of a string for safe display in logs.
// If the string is shorter than visiblePrefix, it is fully masked.
func MaskSecret(s string, visiblePrefix int) string {
	if len(s) <= visiblePrefix {
		return "***"
	}
	return s[:visiblePrefix] + "***"
}
