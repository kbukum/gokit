package util

import (
	"testing"
)

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trims whitespace", "  hello  ", "hello"},
		{"removes control chars", "hello\x00world", "helloworld"},
		{"removes tabs and newlines", "line1\n\tline2", "line1line2"},
		{"empty string", "", ""},
		{"no changes needed", "clean", "clean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeString(tc.input)
			if got != tc.want {
				t.Errorf("SanitizeString(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizeEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips double quotes", `"value"`, "value"},
		{"strips single quotes", `'value'`, "value"},
		{"trims whitespace", "  value  ", "value"},
		{"strips quotes and trims", `  "value"  `, "value"},
		{"no quotes", "value", "value"},
		{"empty string", "", ""},
		{"mismatched quotes", `"value'`, `"value'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeEnvValue(tc.input)
			if got != tc.want {
				t.Errorf("SanitizeEnvValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsSafeString(t *testing.T) {
	tests := []struct {
		name string
		input string
		want  bool
	}{
		{"safe string", "hello world", true},
		{"SQL injection single quote", "'; DROP TABLE users;", false},
		{"SQL injection UNION", "1 UNION SELECT * FROM users", false},
		{"SQL injection double dash", "admin--", false},
		{"XSS script tag", "<script>alert('xss')</script>", false},
		{"XSS javascript", "javascript:alert(1)", false},
		{"XSS event handler", "onerror=alert(1)", false},
		{"safe with numbers", "user123", true},
		{"safe email", "user@example.com", true},
		{"empty string", "", true},
		{"SQL DELETE", "DELETE FROM users", false},
		{"SQL INSERT", "INSERT INTO users", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSafeString(tc.input)
			if got != tc.want {
				t.Errorf("IsSafeString(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
