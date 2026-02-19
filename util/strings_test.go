package util

import "testing"

func TestStringInSlice(t *testing.T) {
	tests := []struct {
		s    string
		list []string
		want bool
	}{
		{"a", []string{"a", "b", "c"}, true},
		{"d", []string{"a", "b", "c"}, false},
		{"", []string{"a", ""}, true},
		{"x", []string{}, false},
	}
	for _, tc := range tests {
		if got := StringInSlice(tc.s, tc.list); got != tc.want {
			t.Errorf("StringInSlice(%q, %v) = %v, want %v", tc.s, tc.list, got, tc.want)
		}
	}
}

func TestCoalesce(t *testing.T) {
	if got := Coalesce("", "", "hello", "world"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := Coalesce(0, 0, 42); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := Coalesce("", ""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
