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
