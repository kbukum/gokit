package util

import (
	"errors"
	"reflect"
	"testing"
)

func TestGroupBy(t *testing.T) {
	t.Parallel()
	got := GroupBy([]int{1, 2, 3, 4}, func(n int) int { return n % 2 })
	if !reflect.DeepEqual(got[0], []int{2, 4}) || !reflect.DeepEqual(got[1], []int{1, 3}) {
		t.Fatalf("GroupBy = %v", got)
	}
}

func TestIndexBy(t *testing.T) {
	t.Parallel()
	got := IndexBy([]string{"aa", "b", "cc"}, func(s string) int { return len(s) })
	if got[1] != "b" || got[2] != "cc" {
		t.Fatalf("IndexBy = %v", got)
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()
	matched, rest := Partition([]int{1, 2, 3, 4}, func(n int) bool { return n > 2 })
	if !reflect.DeepEqual(matched, []int{3, 4}) || !reflect.DeepEqual(rest, []int{1, 2}) {
		t.Fatalf("Partition = %v / %v", matched, rest)
	}
}

func TestChunk(t *testing.T) {
	t.Parallel()
	got := Chunk([]int{1, 2, 3, 4, 5}, 2)
	want := [][]int{{1, 2}, {3, 4}, {5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Chunk = %v", got)
	}
	if Chunk([]int{1}, 0) != nil {
		t.Fatal("non-positive size should yield nil")
	}
}

func TestFindDuplicatesBy(t *testing.T) {
	t.Parallel()
	got := FindDuplicatesBy([]string{"a", "b", "a", "c", "b"}, func(s string) string { return s })
	if len(got) != 2 {
		t.Fatalf("FindDuplicatesBy = %v", got)
	}
}

func TestEnsureUniqueBy(t *testing.T) {
	t.Parallel()
	if err := EnsureUniqueBy([]int{1, 2, 3}, func(n int) int { return n }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := EnsureUniqueBy([]int{1, 2, 2}, func(n int) int { return n })
	var dup *DuplicateKeyError[int]
	if !errors.As(err, &dup) || dup.Key != 2 {
		t.Fatalf("expected DuplicateKeyError{2}, got %v", err)
	}
}
