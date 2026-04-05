package util

import (
	"sort"
	"testing"
)

// ── Contains extended ───────────────────────────────────────────────────

func TestContains_SingleElement(t *testing.T) {
	if !Contains([]int{42}, 42) {
		t.Error("expected to find 42 in single-element slice")
	}
	if Contains([]int{42}, 99) {
		t.Error("expected not to find 99 in single-element slice")
	}
}

func TestContains_LargeSlice(t *testing.T) {
	large := make([]int, 10000)
	for i := range large {
		large[i] = i
	}
	if !Contains(large, 9999) {
		t.Error("expected to find last element in large slice")
	}
	if !Contains(large, 0) {
		t.Error("expected to find first element in large slice")
	}
	if Contains(large, 10000) {
		t.Error("expected not to find out-of-range element")
	}
}

func TestContains_BoolType(t *testing.T) {
	if !Contains([]bool{true, false}, true) {
		t.Error("expected to find true")
	}
	if !Contains([]bool{false}, false) {
		t.Error("expected to find false")
	}
}

func TestContains_NilSlice(t *testing.T) {
	var s []int
	if Contains(s, 1) {
		t.Error("expected not to find value in nil slice")
	}
}

// ── Filter extended ─────────────────────────────────────────────────────

func TestFilter_AllMatch(t *testing.T) {
	result := Filter([]int{2, 4, 6}, func(n int) bool { return n%2 == 0 })
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
}

func TestFilter_NoneMatch(t *testing.T) {
	result := Filter([]int{1, 3, 5}, func(n int) bool { return n%2 == 0 })
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestFilter_NilSlice(t *testing.T) {
	var s []int
	result := Filter(s, func(n int) bool { return true })
	if len(result) != 0 {
		t.Errorf("expected empty result for nil slice, got %d", len(result))
	}
}

func TestFilter_LargeSlice(t *testing.T) {
	large := make([]int, 100000)
	for i := range large {
		large[i] = i
	}
	evens := Filter(large, func(n int) bool { return n%2 == 0 })
	if len(evens) != 50000 {
		t.Errorf("expected 50000 evens, got %d", len(evens))
	}
}

func TestFilter_Strings(t *testing.T) {
	words := []string{"apple", "banana", "avocado", "cherry", "apricot"}
	aWords := Filter(words, func(s string) bool { return s[0] == 'a' })
	if len(aWords) != 3 {
		t.Errorf("expected 3 words starting with 'a', got %d", len(aWords))
	}
}

func TestFilter_PreservesOrder(t *testing.T) {
	result := Filter([]int{5, 3, 1, 4, 2}, func(n int) bool { return n > 2 })
	expected := []int{5, 3, 4}
	if len(result) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

// ── Map extended ────────────────────────────────────────────────────────

func TestMap_EmptySlice(t *testing.T) {
	result := Map([]int{}, func(n int) string { return "" })
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestMap_NilSlice(t *testing.T) {
	var s []int
	result := Map(s, func(n int) int { return n * 2 })
	if len(result) != 0 {
		t.Errorf("expected empty result for nil slice, got %d", len(result))
	}
}

func TestMap_LargeSlice(t *testing.T) {
	large := make([]int, 100000)
	for i := range large {
		large[i] = i
	}
	result := Map(large, func(n int) int { return n + 1 })
	if len(result) != 100000 {
		t.Fatalf("expected 100000 results, got %d", len(result))
	}
	if result[0] != 1 || result[99999] != 100000 {
		t.Errorf("unexpected values: first=%d, last=%d", result[0], result[99999])
	}
}

func TestMap_IntToString(t *testing.T) {
	result := Map([]int{1, 2, 3}, func(n int) string {
		return string(rune('0' + n))
	})
	expected := []string{"1", "2", "3"}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestMap_SingleElement(t *testing.T) {
	result := Map([]int{5}, func(n int) int { return n * n })
	if len(result) != 1 || result[0] != 25 {
		t.Errorf("expected [25], got %v", result)
	}
}

// ── Unique extended ─────────────────────────────────────────────────────

func TestUnique_AllDuplicates(t *testing.T) {
	result := Unique([]int{7, 7, 7, 7, 7})
	if len(result) != 1 || result[0] != 7 {
		t.Errorf("expected [7], got %v", result)
	}
}

func TestUnique_SingleElement(t *testing.T) {
	result := Unique([]int{42})
	if len(result) != 1 || result[0] != 42 {
		t.Errorf("expected [42], got %v", result)
	}
}

func TestUnique_LargeInput(t *testing.T) {
	large := make([]int, 100000)
	for i := range large {
		large[i] = i % 100 // only 100 unique values
	}
	result := Unique(large)
	if len(result) != 100 {
		t.Errorf("expected 100 unique values, got %d", len(result))
	}
}

func TestUnique_PreservesOrder(t *testing.T) {
	result := Unique([]string{"c", "a", "b", "a", "c"})
	expected := []string{"c", "a", "b"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestUnique_NilSlice(t *testing.T) {
	var s []int
	result := Unique(s)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil slice, got %d", len(result))
	}
}

func TestUnique_Bools(t *testing.T) {
	result := Unique([]bool{true, false, true, false, true})
	if len(result) != 2 {
		t.Errorf("expected 2 unique bools, got %d", len(result))
	}
}

// ── Keys extended ───────────────────────────────────────────────────────

func TestKeys_SingleEntry(t *testing.T) {
	keys := Keys(map[string]int{"only": 1})
	if len(keys) != 1 || keys[0] != "only" {
		t.Errorf("expected [only], got %v", keys)
	}
}

func TestKeys_IntKeys(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	keys := Keys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	sort.Ints(keys)
	expected := []int{1, 2, 3}
	for i, v := range keys {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestKeys_NilMap(t *testing.T) {
	var m map[string]int
	keys := Keys(m)
	if len(keys) != 0 {
		t.Errorf("expected empty keys for nil map, got %d", len(keys))
	}
}

// ── Values extended ─────────────────────────────────────────────────────

func TestValues_SingleEntry(t *testing.T) {
	vals := Values(map[string]int{"only": 42})
	if len(vals) != 1 || vals[0] != 42 {
		t.Errorf("expected [42], got %v", vals)
	}
}

func TestValues_NilMap(t *testing.T) {
	var m map[string]int
	vals := Values(m)
	if len(vals) != 0 {
		t.Errorf("expected empty values for nil map, got %d", len(vals))
	}
}

func TestValues_LargeMap(t *testing.T) {
	m := make(map[int]int, 10000)
	for i := 0; i < 10000; i++ {
		m[i] = i * 2
	}
	vals := Values(m)
	if len(vals) != 10000 {
		t.Errorf("expected 10000 values, got %d", len(vals))
	}
}
