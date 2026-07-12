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

// ── ParseSize extended ──────────────────────────────────────────────────

func TestParseSize_ZeroValues(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0MB", 0},
		{"0KB", 0},
		{"0GB", 0},
		{"0", 0},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseSize(tc.input, -1); got != tc.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseSize_LargeValues(t *testing.T) {
	// 100GB
	got := ParseSize("100GB", 0)
	want := int64(100) * 1024 * 1024 * 1024
	if got != want {
		t.Errorf("ParseSize(100GB) = %d, want %d", got, want)
	}
}

func TestParseSize_OnlySuffix(t *testing.T) {
	// "MB" with no number should return default
	got := ParseSize("MB", 42)
	if got != 42 {
		t.Errorf("ParseSize(MB) = %d, want default 42", got)
	}
}

func TestParseSize_WhitespaceOnly(t *testing.T) {
	got := ParseSize("   ", 99)
	if got != 99 {
		t.Errorf("ParseSize(whitespace) = %d, want default 99", got)
	}
}

func TestParseSize_MixedCase(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"10Mb", 10 * 1024 * 1024},
		{"10mB", 10 * 1024 * 1024},
		{"10Kb", 10 * 1024},
		{"10kB", 10 * 1024},
		{"10Gb", 10 * 1024 * 1024 * 1024},
		{"10gB", 10 * 1024 * 1024 * 1024},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseSize(tc.input, 0); got != tc.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseSize_NegativeNumber(t *testing.T) {
	// Sscanf(%d) parses negative integers, so -10MB yields -10 * MB
	got := ParseSize("-10MB", 777)
	want := int64(-10) * 1024 * 1024
	if got != want {
		t.Errorf("ParseSize(-10MB) = %d, want %d", got, want)
	}
}

func TestParseSize_FloatingPoint(t *testing.T) {
	// Sscanf(%d) reads integer prefix of "1.5", i.e. 1
	got := ParseSize("1.5MB", 777)
	want := int64(1) * 1024 * 1024
	if got != want {
		t.Errorf("ParseSize(1.5MB) = %d, want %d", got, want)
	}
}

func TestParseSize_AlphaAndSpecial(t *testing.T) {
	defaultVal := int64(777)
	tests := []struct {
		name  string
		input string
	}{
		{"alpha", "abc"},
		{"special chars", "!@#$"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseSize(tc.input, defaultVal)
			if got != defaultVal {
				t.Errorf("ParseSize(%q) = %d, want default %d", tc.input, got, defaultVal)
			}
		})
	}
}

// ── MaskSecret extended ────────────────────────────────────────────────

func TestMaskSecret_ZeroPrefix(t *testing.T) {
	got := MaskSecret("secret", 0)
	if got != "***" {
		t.Errorf("MaskSecret with prefix=0: got %q, want %q", got, "***")
	}
}

func TestMaskSecret_VeryLongString(t *testing.T) {
	long := make([]byte, 10000)
	for i := range long {
		long[i] = 'a'
	}
	s := string(long)
	got := MaskSecret(s, 5)
	if got != "aaaaa***" {
		t.Errorf("expected 'aaaaa***', got %q", got)
	}
}

func TestMaskSecret_UnicodeString(t *testing.T) {
	// Unicode: "héllo" has byte length > rune count
	s := "héllo world secret"
	got := MaskSecret(s, 4)
	// MaskSecret works on byte length, so s[:4] = "hél" (é is 2 bytes)
	// The result will be the first 4 bytes + "***"
	if len(got) < 4 {
		t.Errorf("expected masked string with prefix, got %q", got)
	}
	if got[len(got)-3:] != "***" {
		t.Errorf("expected '***' suffix, got %q", got)
	}
}

func TestMaskSecret_EmojiString(t *testing.T) {
	s := "🔑secret-key-12345"
	got := MaskSecret(s, 4)
	// Emoji "🔑" is 4 bytes in UTF-8, so s[:4] = "🔑"
	if got != "🔑***" {
		t.Errorf("expected '🔑***', got %q", got)
	}
}

func TestMaskSecret_ExactlyPrefixPlusOne(t *testing.T) {
	got := MaskSecret("abcde", 4)
	if got != "abcd***" {
		t.Errorf("expected 'abcd***', got %q", got)
	}
}

func TestMaskSecret_SingleChar(t *testing.T) {
	got := MaskSecret("x", 1)
	if got != "***" {
		t.Errorf("expected '***' for single char with prefix=1, got %q", got)
	}
}
