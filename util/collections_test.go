package util

import "testing"

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []int
		val   int
		want  bool
	}{
		{"found", []int{1, 2, 3}, 2, true},
		{"not found", []int{1, 2, 3}, 4, false},
		{"empty slice", []int{}, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Contains(tc.slice, tc.val); got != tc.want {
				t.Errorf("Contains(%v, %d) = %v, want %v", tc.slice, tc.val, got, tc.want)
			}
		})
	}
}

func TestContainsStrings(t *testing.T) {
	if !Contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected Contains to find 'b'")
	}
	if Contains([]string{"a", "b"}, "z") {
		t.Error("expected Contains to not find 'z'")
	}
}

func TestFilter(t *testing.T) {
	evens := Filter([]int{1, 2, 3, 4, 5, 6}, func(n int) bool { return n%2 == 0 })
	if len(evens) != 3 {
		t.Fatalf("expected 3 evens, got %d", len(evens))
	}
	for _, v := range evens {
		if v%2 != 0 {
			t.Errorf("expected even, got %d", v)
		}
	}
}

func TestFilterEmpty(t *testing.T) {
	result := Filter([]int{}, func(n int) bool { return true })
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d elements", len(result))
	}
}

func TestMap(t *testing.T) {
	doubled := Map([]int{1, 2, 3}, func(n int) int { return n * 2 })
	expected := []int{2, 4, 6}
	if len(doubled) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(doubled))
	}
	for i, v := range doubled {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestMapTypeConversion(t *testing.T) {
	lengths := Map([]string{"a", "bb", "ccc"}, func(s string) int { return len(s) })
	expected := []int{1, 2, 3}
	for i, v := range lengths {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestUnique(t *testing.T) {
	result := Unique([]int{1, 2, 2, 3, 1, 4})
	if len(result) != 4 {
		t.Fatalf("expected 4 unique values, got %d", len(result))
	}
	expected := []int{1, 2, 3, 4}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestUniqueEmpty(t *testing.T) {
	result := Unique([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	keys := Keys(m)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if !Contains(keys, "a") || !Contains(keys, "b") {
		t.Errorf("expected keys to contain 'a' and 'b', got %v", keys)
	}
}

func TestKeysEmpty(t *testing.T) {
	keys := Keys(map[string]int{})
	if len(keys) != 0 {
		t.Errorf("expected empty keys, got %d", len(keys))
	}
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	vals := Values(m)
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if !Contains(vals, 1) || !Contains(vals, 2) {
		t.Errorf("expected values to contain 1 and 2, got %v", vals)
	}
}
