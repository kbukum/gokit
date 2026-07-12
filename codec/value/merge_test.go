package value_test

import (
	"reflect"
	"testing"

	"github.com/kbukum/gokit/codec/value"
)

func obj(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

func TestOverlayScalarWinsLast(t *testing.T) {
	t.Parallel()
	got := value.Merge(obj("name", "base", "retries", 1), obj("retries", 5))
	want := obj("name", "base", "retries", 5)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestObjectsMergeRecursively(t *testing.T) {
	t.Parallel()
	got := value.Merge(
		obj("server", obj("host", "a", "port", 1)),
		obj("server", obj("port", 2)),
	)
	want := obj("server", obj("host", "a", "port", 2))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestOverlayAddsNewKeys(t *testing.T) {
	t.Parallel()
	got := value.Merge(obj("a", 1), obj("b", 2))
	want := obj("a", 1, "b", 2)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestArraysReplaceByDefault(t *testing.T) {
	t.Parallel()
	got := value.Merge(obj("ports", []any{1, 2, 3}), obj("ports", []any{9}))
	want := obj("ports", []any{9})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSelectedArraysConcatenate(t *testing.T) {
	t.Parallel()
	strategy := func(key string) value.ArrayStrategy {
		if key == "groups" {
			return value.Concat
		}
		return value.Replace
	}
	got := value.MergeWith(
		obj("groups", []any{obj("name", "a")}),
		obj("groups", []any{obj("name", "b")}),
		strategy,
	)
	want := obj("groups", []any{obj("name", "a"), obj("name", "b")})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestUnselectedArraysReplaceUnderConcatStrategy(t *testing.T) {
	t.Parallel()
	strategy := func(key string) value.ArrayStrategy {
		if key == "groups" {
			return value.Concat
		}
		return value.Replace
	}
	got := value.MergeWith(
		obj("groups", []any{1}, "ports", []any{1, 2}),
		obj("groups", []any{2}, "ports", []any{9}),
		strategy,
	)
	want := obj("groups", []any{1, 2}, "ports", []any{9})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTypeMismatchResolvesToOverlay(t *testing.T) {
	t.Parallel()
	got := value.Merge(obj("x", obj("deep", true)), obj("x", 5))
	want := obj("x", 5)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestMergeDoesNotMutateInputs(t *testing.T) {
	t.Parallel()
	base := obj("a", obj("b", 1))
	overlay := obj("a", obj("c", 2))
	_ = value.Merge(base, overlay)
	if !reflect.DeepEqual(base, obj("a", obj("b", 1))) {
		t.Fatalf("base mutated: %#v", base)
	}
}
