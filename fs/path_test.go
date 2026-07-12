package fs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestValidateRelativePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want error
	}{
		{"simple", "a/b/c", nil},
		{"current dir", "./a/b", nil},
		{"absolute", "/etc/passwd", fs.ErrPathAbsolute},
		{"parent traversal", "a/../../etc", fs.ErrPathParentDir},
		{"leading parent", "../secret", fs.ErrPathParentDir},
	}
	for _, tc := range tests {
		if got := fs.ValidateRelativePath(tc.path); !errors.Is(got, tc.want) {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestSafeJoin(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "srv", "data")
	got, err := fs.SafeJoin(root, "a/b.txt")
	if err != nil {
		t.Fatalf("safe join: %v", err)
	}
	if got != filepath.Join(root, "a", "b.txt") {
		t.Fatalf("got %q", got)
	}
	if _, err := fs.SafeJoin(root, "../etc/passwd"); !errors.Is(err, fs.ErrPathParentDir) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestNormalizeRelativePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
		err  error
	}{
		{"a/./b", filepath.Join("a", "b"), nil},
		{"a/b", filepath.Join("a", "b"), nil},
		{".", ".", nil},
		{"./.", ".", nil},
		{"", "", fs.ErrPathEmpty},
		{"a/../b", "", fs.ErrPathParentDir},
		{"/abs", "", fs.ErrPathAbsolute},
	}
	for _, tc := range tests {
		got, err := fs.NormalizeRelativePath(tc.path)
		if !errors.Is(err, tc.err) || (tc.err == nil && got != tc.want) {
			t.Fatalf("%q: got (%q, %v) want (%q, %v)", tc.path, got, err, tc.want, tc.err)
		}
	}
}

func TestFindInAncestors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	if err := fs.WriteAtomic(filepath.Join(dir, "marker.txt"), []byte("x"), "test"); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := fs.WriteAtomic(filepath.Join(nested, "keep"), []byte("y"), "test"); err != nil {
		t.Fatalf("create nested: %v", err)
	}
	got, ok := fs.FindInAncestors(nested, "marker.txt")
	if !ok {
		t.Fatal("expected to find marker")
	}
	// Resolve both sides to avoid /var vs /private/var symlink differences on macOS.
	wantResolved, _ := filepath.EvalSymlinks(filepath.Join(dir, "marker.txt"))
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved {
		t.Fatalf("got %q want %q", gotResolved, wantResolved)
	}
	if _, ok := fs.FindInAncestors(nested, "missing.txt"); ok {
		t.Fatal("expected missing file to not be found")
	}
}

func TestParentDir(t *testing.T) {
	t.Parallel()
	if parent, ok := fs.ParentDir(filepath.Join("a", "b")); !ok || parent != "a" {
		t.Fatalf("got (%q, %t)", parent, ok)
	}
	if _, ok := fs.ParentDir(string(filepath.Separator)); ok {
		t.Fatal("root should have no parent")
	}
}

// FuzzValidateRelativePath ensures no input is misclassified as safe when it
// contains traversal, and that validation never panics.
func FuzzValidateRelativePath(f *testing.F) {
	f.Add("a/b")
	f.Add("../etc")
	f.Add("/abs")
	f.Add("a/../b")
	f.Add("")
	f.Fuzz(func(t *testing.T, path string) {
		err := fs.ValidateRelativePath(path)
		if err != nil {
			return
		}
		// A validated path must join under a root without escaping it.
		joined, joinErr := fs.SafeJoin("/root", path)
		if joinErr != nil {
			t.Fatalf("validated path rejected by SafeJoin: %q (%v)", path, joinErr)
		}
		rel, relErr := filepath.Rel("/root", joined)
		if relErr == nil && (rel == ".." || len(rel) >= 3 && rel[0] == '.' && rel[1] == '.' && rel[2] == filepath.Separator) {
			t.Fatalf("validated path escapes root: %q -> %q", path, joined)
		}
	})
}
