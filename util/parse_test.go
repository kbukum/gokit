package util

import "testing"

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input  string
		prefix int
		want   string
	}{
		{"host=localhost user=admin ******", 10, "host=local***"},
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
