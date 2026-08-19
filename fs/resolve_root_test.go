package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootRelativeToRelative(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRootRelativeTo("root", base, "sub")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Canonicalize(sub)
	if got != want {
		t.Fatalf("ResolveRootRelativeTo = %q, want %q", got, want)
	}
}

func TestResolveRootRelativeToEmptyDefaultsToBase(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	got, err := ResolveRootRelativeTo("root", base, "")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Canonicalize(base)
	if got != want {
		t.Fatalf("empty root = %q, want base %q", got, want)
	}
}

func TestResolveRootRelativeToAbsolute(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	other := t.TempDir()
	got, err := ResolveRootRelativeTo("root", base, other)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Canonicalize(other)
	if got != want {
		t.Fatalf("absolute root = %q, want %q", got, want)
	}
}

func TestResolveRootRelativeToMissingErrors(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if _, err := ResolveRootRelativeTo("root", base, "does-not-exist"); err == nil {
		t.Fatal("expected error for missing root")
	}
}
