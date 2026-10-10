package testutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kbukum/gokit/database"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestWithPasswordFileWritesAPrivateSecretFile(t *testing.T) {
	params, err := pgtest.WithPasswordFile(t.TempDir(), database.ConnParams{User: "runtime", Password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if params.Password != "" || params.User != "runtime" {
		t.Fatalf("params = %+v", params)
	}
	info, err := os.Stat(params.PasswordFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	data, err := os.ReadFile(params.PasswordFile)
	if err != nil || string(data) != "s3cret" {
		t.Fatalf("contents = %q, %v", data, err)
	}
}

func TestWithPasswordFileRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		dir    string
		params database.ConnParams
	}{
		"no directory": {"", database.ConnParams{Password: "x"}},
		"no password":  {dir, database.ConnParams{}},
		"already file": {dir, database.ConnParams{Password: "x", PasswordFile: "/run/secret"}},
		"oversized":    {dir, database.ConnParams{Password: strings.Repeat("x", 4<<10+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pgtest.WithPasswordFile(tc.dir, tc.params); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if _, err := pgtest.WithPasswordFile(dir+"/missing", database.ConnParams{Password: "x"}); err == nil {
		t.Fatal("missing directory accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected input left files: %v %v", entries, err)
	}
}
