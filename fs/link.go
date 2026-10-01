package fs

import (
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// HardLink creates a hard link at linkPath pointing to the same inode as original.
func HardLink(original, linkPath string) error {
	if err := os.Link(original, linkPath); err != nil {
		code := osErrorCode(err)
		return apperrors.New(code,
			"failed to create hard link",
		).WithCause(err)
	}
	return nil
}

// ReadLink returns the target of the symbolic link at path.
func ReadLink(path string) (string, error) {
	target, err := os.Readlink(path)
	if err != nil {
		code := osErrorCode(err)
		return "", apperrors.New(code,
			"failed to read link").WithCause(err)
	}
	return target, nil
}

// SymlinkFile creates a symbolic link at linkPath pointing to original.
func SymlinkFile(original, linkPath string) error {
	if err := os.Symlink(original, linkPath); err != nil {
		code := osErrorCode(err)
		return apperrors.New(code,
			"failed to create symlink",
		).WithCause(err)
	}
	return nil
}

// SymlinkDir creates a symbolic link at linkPath pointing to the directory original.
// On POSIX systems a directory symlink is created identically to a file symlink.
func SymlinkDir(original, linkPath string) error {
	return SymlinkFile(original, linkPath)
}
