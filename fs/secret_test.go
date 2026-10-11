package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
)

func TestReadSecret(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "secret")
	for _, ending := range []string{"", "\n", "\r\n"} {
		if err := os.WriteFile(path, []byte("sentinel"+ending), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := fs.ReadSecret(path, 8, 8)
		if err != nil || string(got) != "sentinel" {
			t.Fatal("secret read or trailing line ending failed", err)
		}
	}
	for _, bounds := range [][2]int{{0, 8}, {9, 8}, {1, fs.MaxSecretBytes + 1}, {9, 16}, {1, 1}} {
		got, err := fs.ReadSecret(path, bounds[0], bounds[1])
		if err == nil || got != nil || strings.Contains(err.Error(), "sentinel") {
			t.Fatal("invalid secret did not fail safely")
		}
	}
	if _, err := fs.ReadSecret(path, 1, 1); !errors.Is(err, fs.ErrFileTooLarge) {
		t.Fatal("size failure lost its cause", err)
	}
	for _, name := range []string{"", path + "-missing"} {
		if _, err := fs.ReadSecret(name, 1, 8); err == nil {
			t.Fatal("missing path accepted")
		}
	}
}
