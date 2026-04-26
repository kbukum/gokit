package logger

import (
	"fmt"
	"regexp"
	"strings"
)

// Masker is the interface for sensitive data masking adapters.
type Masker interface {
	MaskValue(key, value string) string
}

// MaskingConfig contains configuration for sensitive data masking.
type MaskingConfig struct {
	Enabled       bool     `yaml:"enabled" mapstructure:"enabled"`
	FieldNames    []string `yaml:"field_names" mapstructure:"field_names"`       // Additional field names to mask
	ValuePatterns []string `yaml:"value_patterns" mapstructure:"value_patterns"` // Additional regex patterns
	Replacement   string   `yaml:"replacement" mapstructure:"replacement"`
	PreserveLast  int      `yaml:"preserve_last" mapstructure:"preserve_last"` // Preserve last N chars for partial masking
}

// valuePattern pairs a compiled regexp with its replacement function.
type valuePattern struct {
	re      *regexp.Regexp
	replace func(match string) string
}

// DefaultMasker provides field-name and value-pattern based sensitive data masking.
// It is safe for concurrent use after construction (all fields are read-only).
type DefaultMasker struct {
	fieldNames    map[string]struct{}
	valuePatterns []valuePattern
	replacement   string
	preserveLast  int
}

// defaultFieldNames are always treated as sensitive regardless of value.
var defaultFieldNames = []string{
	"password", "secret", "token", "api_key", "apikey", "api-key",
	"authorization", "auth_token", "access_token", "refresh_token",
	"private_key", "ssn", "credit_card", "card_number", "cvv", "pin",
}

// NewDefaultMasker creates a DefaultMasker from the given MaskingConfig.
// All regexps are compiled at construction time via regexp.MustCompile.
func NewDefaultMasker(cfg MaskingConfig) *DefaultMasker {
	replacement := cfg.Replacement
	if replacement == "" {
		replacement = "***REDACTED***"
	}

	// Build the field name lookup set.
	names := make(map[string]struct{}, len(defaultFieldNames)+len(cfg.FieldNames))
	for _, n := range defaultFieldNames {
		names[strings.ToLower(n)] = struct{}{}
	}
	for _, n := range cfg.FieldNames {
		names[strings.ToLower(n)] = struct{}{}
	}

	// Build the value patterns (defaults + user-supplied).
	patterns := buildDefaultValuePatterns()
	for _, p := range cfg.ValuePatterns {
		re := regexp.MustCompile(p)
		rep := replacement
		patterns = append(patterns, valuePattern{
			re:      re,
			replace: func(match string) string { return rep },
		})
	}

	return &DefaultMasker{
		fieldNames:    names,
		valuePatterns: patterns,
		replacement:   replacement,
		preserveLast:  cfg.PreserveLast,
	}
}

// buildDefaultValuePatterns returns the built-in value-pattern matchers.
func buildDefaultValuePatterns() []valuePattern {
	return []valuePattern{
		// JWT (must come before generic hex to avoid false matches)
		{
			re:      regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]+`),
			replace: func(string) string { return "[JWT_REDACTED]" },
		},
		// Bearer token
		{
			re:      regexp.MustCompile(`(?i)Bearer\s+[a-zA-Z0-9._~+/=-]+`),
			replace: func(string) string { return "Bearer [REDACTED]" },
		},
		// AWS Access Key
		{
			re:      regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			replace: func(string) string { return "[AWS_KEY_REDACTED]" },
		},
		// Credit Card (with optional spaces/dashes) — preserve last 4 digits
		{
			re: regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),
			replace: func(match string) string {
				digits := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, match)
				if len(digits) >= 4 {
					return "****-****-****-" + digits[len(digits)-4:]
				}
				return "****-****-****-****"
			},
		},
		// SSN
		{
			re:      regexp.MustCompile(`\b\d{3}-?\d{2}-?\d{4}\b`),
			replace: func(string) string { return "***-**-****" },
		},
		// Email
		{
			re:      regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
			replace: func(string) string { return "***@***.***" },
		},
		// Generic hex secret (32+ chars)
		{
			re:      regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`),
			replace: func(string) string { return "[HEX_REDACTED]" },
		},
	}
}

// MaskValue masks a single value based on its field key and content.
// If the key matches a sensitive field name, the full replacement is returned.
// Otherwise each value pattern is tested and the first match wins.
func (m *DefaultMasker) MaskValue(key, value string) string {
	// 1. Field-name match (case-insensitive).
	if _, ok := m.fieldNames[strings.ToLower(key)]; ok {
		if m.preserveLast > 0 && len(value) > m.preserveLast {
			return m.replacement + value[len(value)-m.preserveLast:]
		}
		return m.replacement
	}

	// 2. Value-pattern match.
	for _, vp := range m.valuePatterns {
		if vp.re.MatchString(value) {
			return vp.re.ReplaceAllStringFunc(value, vp.replace)
		}
	}

	// 3. No match — pass through.
	return value
}

// MaskFields masks all values in a map, returning a new map.
// String values are masked directly. Non-string values are converted to a
// string representation, checked for sensitive patterns, and replaced with
// the masked string if a pattern matches.
func (m *DefaultMasker) MaskFields(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return nil
	}
	out := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		str, isStr := v.(string)
		if !isStr {
			str = fmt.Sprintf("%v", v)
		}
		masked := m.MaskValue(k, str)
		if masked != str {
			out[k] = masked
		} else {
			out[k] = v
		}
	}
	return out
}
