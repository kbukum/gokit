package util

import "testing"

func TestPtr(t *testing.T) {
	v := 42
	p := Ptr(v)
	if *p != 42 {
		t.Errorf("expected *p=42, got %d", *p)
	}

	s := Ptr("hello")
	if *s != "hello" {
		t.Errorf("expected *s=hello, got %s", *s)
	}
}

func TestDeref(t *testing.T) {
	v := 42
	if Deref(&v) != 42 {
		t.Error("expected Deref to return 42")
	}

	var p *int
	if Deref(p) != 0 {
		t.Error("expected Deref of nil to return zero value")
	}

	s := "hello"
	if Deref(&s) != "hello" {
		t.Error("expected Deref to return hello")
	}

	var sp *string
	if Deref(sp) != "" {
		t.Error("expected Deref of nil string pointer to return empty string")
	}
}

// ── Ptr extended ────────────────────────────────────────────────────────

func TestPtr_Bool(t *testing.T) {
	p := Ptr(true)
	if *p != true {
		t.Errorf("expected *p=true, got %v", *p)
	}
	p2 := Ptr(false)
	if *p2 != false {
		t.Errorf("expected *p2=false, got %v", *p2)
	}
}

func TestPtr_Float64(t *testing.T) {
	p := Ptr(3.14)
	if *p != 3.14 {
		t.Errorf("expected *p=3.14, got %f", *p)
	}
}

func TestPtr_ZeroValue(t *testing.T) {
	p := Ptr(0)
	if *p != 0 {
		t.Errorf("expected *p=0, got %d", *p)
	}
	sp := Ptr("")
	if *sp != "" {
		t.Errorf("expected *sp='', got %q", *sp)
	}
}

type testStruct struct {
	Name string
	Age  int
}

func TestPtr_Struct(t *testing.T) {
	s := testStruct{Name: "Alice", Age: 30}
	p := Ptr(s)
	if p.Name != "Alice" || p.Age != 30 {
		t.Errorf("expected {Alice, 30}, got %+v", *p)
	}
	// Modifying p should not affect the original
	p.Name = "Bob"
	if s.Name != "Alice" {
		t.Error("Ptr should create a copy, not reference original")
	}
}

func TestPtr_Slice(t *testing.T) {
	s := []int{1, 2, 3}
	p := Ptr(s)
	if len(*p) != 3 {
		t.Errorf("expected slice of length 3, got %d", len(*p))
	}
}

// ── Deref extended ──────────────────────────────────────────────────────

func TestDeref_Bool(t *testing.T) {
	b := true
	if Deref(&b) != true {
		t.Error("expected Deref to return true")
	}

	var bp *bool
	if Deref(bp) != false {
		t.Error("expected Deref of nil bool pointer to return false")
	}
}

func TestDeref_Float64(t *testing.T) {
	f := 3.14
	if Deref(&f) != 3.14 {
		t.Errorf("expected 3.14, got %f", Deref(&f))
	}

	var fp *float64
	if Deref(fp) != 0.0 {
		t.Error("expected Deref of nil float64 pointer to return 0.0")
	}
}

func TestDeref_Struct(t *testing.T) {
	s := testStruct{Name: "Alice", Age: 30}
	got := Deref(&s)
	if got.Name != "Alice" || got.Age != 30 {
		t.Errorf("expected {Alice, 30}, got %+v", got)
	}

	var sp *testStruct
	zero := Deref(sp)
	if zero.Name != "" || zero.Age != 0 {
		t.Errorf("expected zero struct, got %+v", zero)
	}
}

func TestPtr_Deref_RoundTrip(t *testing.T) {
	original := 42
	result := Deref(Ptr(original))
	if result != original {
		t.Errorf("round trip failed: got %d, want %d", result, original)
	}

	origStr := "hello"
	resultStr := Deref(Ptr(origStr))
	if resultStr != origStr {
		t.Errorf("round trip failed: got %q, want %q", resultStr, origStr)
	}
}
