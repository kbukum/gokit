package util

import (
	"reflect"
	"testing"
)

func TestDeepMergeShallow(t *testing.T) {
	base := map[string]any{"a": 1, "b": 2}
	over := map[string]any{"b": 3, "c": 4}
	got := DeepMerge(base, over)
	want := map[string]any{"a": 1, "b": 3, "c": 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeNested(t *testing.T) {
	base := map[string]any{"db": map[string]any{"host": "localhost", "port": 5432}}
	over := map[string]any{"db": map[string]any{"port": 3306, "name": "mydb"}}
	got := DeepMerge(base, over)
	want := map[string]any{"db": map[string]any{"host": "localhost", "port": 3306, "name": "mydb"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeOverrideNonObject(t *testing.T) {
	base := map[string]any{"a": 1}
	over := map[string]any{"a": []int{1, 2, 3}}
	got := DeepMerge(base, over)
	want := map[string]any{"a": []int{1, 2, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeEmptyBase(t *testing.T) {
	got := DeepMerge(map[string]any{}, map[string]any{"a": 1})
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeEmptyOverride(t *testing.T) {
	got := DeepMerge(map[string]any{"a": 1}, map[string]any{})
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeDeeplyNested(t *testing.T) {
	base := map[string]any{"l1": map[string]any{"l2": map[string]any{"l3": map[string]any{"a": 1}}}}
	over := map[string]any{"l1": map[string]any{"l2": map[string]any{"l3": map[string]any{"b": 2}}}}
	got := DeepMerge(base, over)
	want := map[string]any{"l1": map[string]any{"l2": map[string]any{"l3": map[string]any{"a": 1, "b": 2}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDeepMergeDoesNotMutateInputs(t *testing.T) {
	base := map[string]any{"a": 1}
	over := map[string]any{"b": 2}
	_ = DeepMerge(base, over)
	if len(base) != 1 || base["a"] != 1 {
		t.Error("base was mutated")
	}
	if len(over) != 1 || over["b"] != 2 {
		t.Error("override was mutated")
	}
}
