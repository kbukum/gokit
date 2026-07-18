package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	apperrors "github.com/kbukum/gokit/errors"
)

// CanRead reports whether the current process can open path for reading. A permission-denied
// or not-found result yields false, not an error; any other failure is returned.
func CanRead(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, accessError("check read access", path, err)
	}
	_ = file.Close()
	return true, nil
}

// CanWrite reports whether the current process can write to the file or directory at path.
// Directories are probed with a create-new temp file that is removed afterward. A permission-denied
// or not-found result yields false.
func CanWrite(path string) (bool, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return canWriteDir(path)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, accessError("check write access", path, err)
	}
	_ = file.Close()
	return true, nil
}

func canWriteDir(path string) (bool, error) {
	probe := SiblingTempPath(filepath.Join(path, ".probe"), "gokit-fs-permission", ".tmp")
	file, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, accessError("check directory write access", path, err)
	}
	_ = file.Close()
	_ = os.Remove(probe)
	return true, nil
}

// IsReadonly reports whether path's permissions mark it read-only for the owner.
func IsReadonly(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, accessError("read permissions", path, err)
	}
	return info.Mode().Perm()&0o200 == 0, nil
}

func accessError(context, path string, err error) error {
	code, status := osErrorCode(err)
	return apperrors.New(code,
		fmt.Sprintf("failed to %s for '%s': %v", context, path, err), status).WithCause(err)
}
