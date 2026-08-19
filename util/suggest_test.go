package util

import "testing"

func TestNearestFindsClosest(t *testing.T) {
	t.Parallel()
	candidates := []string{"format", "test", "lint"}
	got, ok := Nearest("fmt", candidates)
	if !ok || got != "format" {
		t.Fatalf("Nearest(fmt) = (%q, %v), want format", got, ok)
	}
}

func TestNearestTypo(t *testing.T) {
	t.Parallel()
	got, ok := Nearest("buld", []string{"build", "test"})
	if !ok || got != "build" {
		t.Fatalf("Nearest(buld) = (%q, %v), want build", got, ok)
	}
}

func TestNearestTransposition(t *testing.T) {
	t.Parallel()
	got, ok := Nearest("tset", []string{"test", "rest"})
	if !ok || got != "test" {
		t.Fatalf("Nearest(tset) = (%q, %v), want test", got, ok)
	}
}

func TestNearestNoneCloseEnough(t *testing.T) {
	t.Parallel()
	if _, ok := Nearest("zzzzzz", []string{"build", "test"}); ok {
		t.Fatal("expected no match for unrelated input")
	}
}

func TestNearestWithinDistance(t *testing.T) {
	t.Parallel()
	// "abcd" is not a subsequence of "ax", and their length gap exceeds 1.
	if _, ok := NearestWithin("ax", []string{"abcd"}, 1); ok {
		t.Fatal("distant candidate should not match at maxDistance 1")
	}
}

func TestNearestCaseInsensitive(t *testing.T) {
	t.Parallel()
	got, ok := Nearest("BUILD", []string{"build", "test"})
	if !ok || got != "build" {
		t.Fatalf("case-insensitive Nearest = (%q, %v)", got, ok)
	}
}
