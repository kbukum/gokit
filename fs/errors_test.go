package fs_test

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// requireNotFound asserts err is an AppError typed as not-found (404), the
// contract every fs helper that inspects a caller-provided path must honor for
// a missing path.
func requireNotFound(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.ErrCodeNotFound {
		t.Errorf("code = %s, want %s", appErr.Code, apperrors.ErrCodeNotFound)
	}
	if appErr.HTTPStatus != http.StatusNotFound {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusNotFound)
	}
}

func TestMetadataMissingPathIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := fs.Metadata(filepath.Join(t.TempDir(), "missing"))
	requireNotFound(t, err)
}

func TestReadDirMissingPathIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := fs.ReadDir(filepath.Join(t.TempDir(), "missing"))
	requireNotFound(t, err)
}

func TestIsReadonlyMissingPathIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := fs.IsReadonly(filepath.Join(t.TempDir(), "missing"))
	requireNotFound(t, err)
}

func TestCanonicalizeMissingPathIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := fs.Canonicalize(filepath.Join(t.TempDir(), "missing"))
	requireNotFound(t, err)
}
