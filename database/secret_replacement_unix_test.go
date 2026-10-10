//go:build unix

package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSecretsFollowMountedSymlinkReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, second, path := filepath.Join(dir, "first"), filepath.Join(dir, "second"), filepath.Join(dir, "mounted")
	for name, contents := range map[string]string{first: "synthetic-first", second: "synthetic-second"} {
		if err := os.WriteFile(name, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(first, path); err != nil {
		t.Fatal(err)
	}
	one, err := (FileSecrets{}).ReadSecret(t.Context(), path)
	if err != nil || one.Expose() != "synthetic-first" {
		t.Fatal("mounted secret could not be read")
	}
	replacement := filepath.Join(dir, "replacement")
	if err := os.Symlink(second, replacement); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	two, err := (FileSecrets{}).ReadSecret(t.Context(), path)
	if err != nil || two.Expose() != "synthetic-second" {
		t.Fatal("explicit reread retained the old mounted secret")
	}
}
