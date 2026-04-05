package util

import "testing"

// ── Coalesce extended ───────────────────────────────────────────────────

func TestCoalesce_SingleValue(t *testing.T) {
	if got := Coalesce("hello"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestCoalesce_SingleZeroValue(t *testing.T) {
	if got := Coalesce(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestCoalesce_NoArgs(t *testing.T) {
	if got := Coalesce[string](); got != "" {
		t.Errorf("expected empty string for no args, got %q", got)
	}
	if got := Coalesce[int](); got != 0 {
		t.Errorf("expected 0 for no int args, got %d", got)
	}
}

func TestCoalesce_Bools(t *testing.T) {
	if got := Coalesce(false, true); got != true {
		t.Errorf("expected true, got %v", got)
	}
	if got := Coalesce(false, false); got != false {
		t.Errorf("expected false, got %v", got)
	}
}

func TestCoalesce_ManyZerosBeforeValue(t *testing.T) {
	if got := Coalesce(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 42); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestCoalesce_FloatValues(t *testing.T) {
	if got := Coalesce(0.0, 0.0, 3.14); got != 3.14 {
		t.Errorf("expected 3.14, got %f", got)
	}
}

func TestCoalesce_FirstNonZero(t *testing.T) {
	if got := Coalesce("first", "second", "third"); got != "first" {
		t.Errorf("expected 'first', got %q", got)
	}
}

func TestCoalesce_AllZero(t *testing.T) {
	if got := Coalesce(0, 0, 0); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}
