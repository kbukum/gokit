package testutil

import (
	"errors"
	"os"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
)

const maxPasswordBytes = 4 << 10

// WithPasswordFile writes params' password to a new mode-0600 file in the caller-owned dir, such as t.TempDir(), and
// returns params that reference the file instead, for tests of PasswordFile resolution. The caller removes dir.
func WithPasswordFile(dir string, params database.ConnParams) (database.ConnParams, error) {
	if dir == "" || params.Password == "" || params.PasswordFile != "" || len(params.Password) > maxPasswordBytes {
		return database.ConnParams{}, apperrors.InvalidInput("password", "A directory and one inline password of at most 4 KiB are required")
	}
	file, err := os.CreateTemp(dir, "password-*")
	if err != nil {
		return database.ConnParams{}, err
	}
	path := file.Name()
	err = file.Chmod(0o600)
	if err == nil {
		_, err = file.WriteString(params.Password)
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return database.ConnParams{}, errors.Join(err, os.Remove(path))
	}
	params.Password, params.PasswordFile = "", path
	return params, nil
}
