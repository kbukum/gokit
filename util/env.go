package util

import (
	"os"
	"strings"
)

// GetEnv reads a string environment variable, returning ("", false) when it is unset.
func GetEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// GetEnvNonEmpty reads a string environment variable, treating an empty value as unset.
func GetEnvNonEmpty(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

// GetEnvOr reads a string environment variable, returning fallback when it is unset.
func GetEnvOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetEnvParsed reads an environment variable and parses it with parse, returning
// (zero, false) when the variable is unset or parsing fails.
func GetEnvParsed[T any](key string, parse func(string) (T, error)) (T, bool) {
	var zero T
	value, ok := os.LookupEnv(key)
	if !ok {
		return zero, false
	}
	parsed, err := parse(value)
	if err != nil {
		return zero, false
	}
	return parsed, true
}

// GetEnvBool reads a boolean environment variable, recognizing "true", "1", "yes",
// and "on" as true and "false", "0", "no", and "off" as false (case-insensitively,
// trimmed). It returns fallback when the variable is unset or unrecognized.
func GetEnvBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}
