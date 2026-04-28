package util

import (
	"reflect"
	"testing"
)

func TestDeepMerge_Shallow(t *testing.T) {
	base := map[string]any{"a": 1, "b": 2}
	over := map[string]any{"b": 3, "c": 4}
	got := DeepMerge(base, over)
	want := map[string]any{"a": 1, "b": 3, "c": 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shallow merge = %v, want %v", got, want)
	}
}

func TestDeepMerge_Nested(t *testing.T) {
	base := map[string]any{
		"db": map[string]any{"host": "localhost", "port": 5432},
	}
	over := map[string]any{
		"db": map[string]any{"port": 3306, "name": "mydb"},
	}
	got := DeepMerge(base, over)
	want := map[string]any{
		"db": map[string]any{"host": "localhost", "port": 3306, "name": "mydb"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nested merge = %v, want %v", got, want)
	}
}

func TestDeepMerge_OverrideNonMap(t *testing.T) {
	base := map[string]any{"a": 1}
	over := map[string]any{"a": []int{1, 2, 3}}
	got := DeepMerge(base, over)
	if !reflect.DeepEqual(got["a"], []int{1, 2, 3}) {
		t.Errorf("override non-map = %v, want [1,2,3]", got["a"])
	}
}

func TestDeepMerge_EmptyBase(t *testing.T) {
	got := DeepMerge(map[string]any{}, map[string]any{"a": 1})
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty base = %v, want %v", got, want)
	}
}

func TestDeepMerge_EmptyOverride(t *testing.T) {
	base := map[string]any{"a": 1}
	got := DeepMerge(base, map[string]any{})
	if !reflect.DeepEqual(got, base) {
		t.Errorf("empty override = %v, want %v", got, base)
	}
}

func TestDeepMerge_DoesNotMutate(t *testing.T) {
	base := map[string]any{"a": 1}
	over := map[string]any{"b": 2}
	_ = DeepMerge(base, over)
	if len(base) != 1 {
		t.Error("base was mutated")
	}
	if len(over) != 1 {
		t.Error("override was mutated")
	}
}

func TestDeepMerge_DeeplyNested(t *testing.T) {
	base := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{"a": 1},
			},
		},
	}
	over := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{"b": 2},
			},
		},
	}
	got := DeepMerge(base, over)
	want := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{"a": 1, "b": 2},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("deeply nested = %v, want %v", got, want)
	}
}
