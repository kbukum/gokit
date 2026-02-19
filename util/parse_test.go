package util

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"10MB", 10 * 1024 * 1024},
		{"512KB", 512 * 1024},
		{"2GB", 2 * 1024 * 1024 * 1024},
		{"1024", 1024},
		{"  10MB  ", 10 * 1024 * 1024},
		{"10mb", 10 * 1024 * 1024},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseSize(tc.input, 0); got != tc.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseSize_Default(t *testing.T) {
	defaultVal := int64(5 * 1024 * 1024)
	if got := ParseSize("", defaultVal); got != defaultVal {
		t.Errorf("expected default %d, got %d", defaultVal, got)
	}
	if got := ParseSize("invalid", defaultVal); got != defaultVal {
		t.Errorf("expected default %d for invalid input, got %d", defaultVal, got)
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input  string
		prefix int
		want   string
	}{
		{"host=localhost user=admin password=secret", 10, "host=local***"},
		{"short", 10, "***"},
		{"exactly10!", 10, "***"},
		{"", 5, "***"},
		{"abcdef", 3, "abc***"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := MaskSecret(tc.input, tc.prefix); got != tc.want {
				t.Errorf("MaskSecret(%q, %d) = %q, want %q", tc.input, tc.prefix, got, tc.want)
			}
		})
	}
}
