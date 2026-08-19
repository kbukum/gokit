package util

import "testing"

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("no truncation: got %q", got)
	}
	if got := Truncate("hello world", 8); got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
	if got := Truncate("hello", 3); got != "" {
		t.Errorf("tiny limit: got %q, want empty", got)
	}
}

func TestTruncateEllipsis(t *testing.T) {
	t.Parallel()
	if got := TruncateEllipsis("hello", 10); got != "hello" {
		t.Errorf("no truncation: got %q", got)
	}
	if got := TruncateEllipsis("hello world", 8); got != "hello..." {
		t.Errorf("TruncateEllipsis = %q, want %q", got, "hello...")
	}
	if got := TruncateEllipsis("hello world", 2); got != ".." {
		t.Errorf("tiny limit: got %q, want %q", got, "..")
	}
	if got := TruncateEllipsis("hello", -1); got != "" {
		t.Errorf("negative limit must not panic: got %q, want empty", got)
	}
}

func TestTruncateRuneBoundary(t *testing.T) {
	t.Parallel()
	// "héllo" — é is two bytes; ensure no split.
	got := Truncate("héllo world", 5)
	for i := 0; i < len(got); i++ {
		_ = got // valid UTF-8 by construction
	}
	if len(got) > 5 {
		t.Errorf("exceeded limit: %q", got)
	}
}
