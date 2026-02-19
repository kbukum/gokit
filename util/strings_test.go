package util

import "testing"

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
