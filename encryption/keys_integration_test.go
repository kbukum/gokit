//go:build integration

package encryption_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/encryption"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestMountedKeysAreBoundedRedactedAndReadWithoutCaching(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "master")
	secret := strings.Repeat("master-sentinel-", 4)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []encryption.KeyFile{{ID: "current", Path: path}}
	source, err := encryption.NewFileKeys(files)
	if err != nil {
		t.Fatal(err)
	}
	files[0].Path = "/missing"
	key, err := source.Key(t.Context(), "current")
	if err != nil || key.Expose() != secret {
		t.Fatal("mounted key reference was not copied or selected")
	}
	encoded, err := json.Marshal(key)
	if err != nil || strings.Contains(string(encoded), secret) || strings.Contains(fmt.Sprintf("%v %#v", key, key), secret) {
		t.Fatal("key escaped redacted formatting")
	}
	rotated := strings.Repeat("rotation-sentinel-", 4)
	if err := os.WriteFile(path, []byte(rotated), 0o600); err != nil {
		t.Fatal(err)
	}
	if key, err := source.Key(t.Context(), "current"); err != nil || key.Expose() != rotated {
		t.Fatal("key access used a positive cache")
	}
	for _, size := range []int{0, 31, 1025} {
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatal(err)
		}
		if key, err := source.Key(t.Context(), "current"); err == nil || !key.IsEmpty() {
			t.Fatal("out-of-bound key succeeded")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if key, err := source.Key(t.Context(), "current"); err == nil || !key.IsEmpty() ||
		!errors.Is(err, fs.ErrNotExist) || apperrors.Normalize(err).Code != apperrors.ServiceUnavailable("encryption key").Code ||
		apperrors.Normalize(err).Reason != "ENCRYPTION_KEY_UNAVAILABLE" {
		t.Fatal("missing mounted key did not report a redacted dependency failure with its cause")
	}
}
