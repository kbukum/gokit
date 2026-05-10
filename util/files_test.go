package util

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "c")

	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}
	if !DirExists(path) {
		t.Fatalf("DirExists(%q) = false, want true", path)
	}
	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir() on existing directory failed: %v", err)
	}
}

func TestFileExistsAndDirExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirPath := filepath.Join(root, "dir")
	filePath := filepath.Join(root, "dir", "file.txt")

	if err := EnsureDir(dirPath); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}
	if err := WriteFile(filePath, []byte("hello")); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		fileWant bool
		dirWant  bool
	}{
		{name: "regular file", path: filePath, fileWant: true},
		{name: "directory", path: dirPath, dirWant: true},
		{name: "missing path", path: filepath.Join(root, "missing")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := FileExists(tt.path); got != tt.fileWant {
				t.Fatalf("FileExists(%q) = %v, want %v", tt.path, got, tt.fileWant)
			}
			if got := DirExists(tt.path); got != tt.dirWant {
				t.Fatalf("DirExists(%q) = %v, want %v", tt.path, got, tt.dirWant)
			}
		})
	}
}

func TestWriteFileAndReadFileString(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "nested", "file.txt")
	content := "hello\nworld\n"

	if err := WriteFile(path, []byte(content)); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), 0o644)
	}

	got, err := ReadFileString(path)
	if err != nil {
		t.Fatalf("ReadFileString() failed: %v", err)
	}
	if got != content {
		t.Fatalf("ReadFileString() = %q, want %q", got, content)
	}
}

func TestRemoveAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "tree")
	if err := WriteFile(filepath.Join(target, "nested", "file.txt"), []byte("data")); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	if err := RemoveAll(target); err != nil {
		t.Fatalf("RemoveAll() failed: %v", err)
	}
	if FileExists(filepath.Join(target, "nested", "file.txt")) || DirExists(target) {
		t.Fatal("target still exists after RemoveAll()")
	}
	if err := RemoveAll(target); err != nil {
		t.Fatalf("RemoveAll() on missing path failed: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "nested", "dst.txt")
	content := []byte("copied content")
	modTime := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)

	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatalf("WriteFile(src) failed: %v", err)
	}
	if err := os.Chtimes(src, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(src) failed: %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) failed: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("copied content = %q, want %q", string(data), string(content))
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat(dst) failed: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), 0o600)
	}
	if !info.ModTime().Equal(modTime) {
		t.Fatalf("mod time = %v, want %v", info.ModTime(), modTime)
	}
}

func TestCopyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	nestedDir := filepath.Join(src, "nested")
	filePath := filepath.Join(nestedDir, "file.txt")
	linkPath := filepath.Join(src, "file-link")
	dirTime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	fileTime := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)

	if err := EnsureDir(nestedDir); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}
	if err := os.Chmod(nestedDir, 0o750); err != nil {
		t.Fatalf("Chmod(nestedDir) failed: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o640); err != nil {
		t.Fatalf("WriteFile(file) failed: %v", err)
	}
	if err := os.Chtimes(filePath, fileTime, fileTime); err != nil {
		t.Fatalf("Chtimes(file) failed: %v", err)
	}
	if err := os.Chtimes(nestedDir, dirTime, dirTime); err != nil {
		t.Fatalf("Chtimes(nestedDir) failed: %v", err)
	}
	if err := os.Symlink(filepath.Join("nested", "file.txt"), linkPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Symlink requires elevated privileges on Windows: %v", err)
		}
		t.Fatalf("Symlink() failed: %v", err)
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir() failed: %v", err)
	}

	copiedDir := filepath.Join(dst, "nested")
	copiedFile := filepath.Join(copiedDir, "file.txt")
	copiedLink := filepath.Join(dst, "file-link")

	if !DirExists(copiedDir) {
		t.Fatalf("DirExists(%q) = false, want true", copiedDir)
	}
	if !FileExists(copiedFile) {
		t.Fatalf("FileExists(%q) = false, want true", copiedFile)
	}

	dirInfo, err := os.Stat(copiedDir)
	if err != nil {
		t.Fatalf("Stat(copiedDir) failed: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o750 {
		t.Fatalf("dir permissions = %o, want %o", dirInfo.Mode().Perm(), 0o750)
	}
	if !dirInfo.ModTime().Equal(dirTime) {
		t.Fatalf("dir mod time = %v, want %v", dirInfo.ModTime(), dirTime)
	}

	fileInfo, err := os.Stat(copiedFile)
	if err != nil {
		t.Fatalf("Stat(copiedFile) failed: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o640 {
		t.Fatalf("file permissions = %o, want %o", fileInfo.Mode().Perm(), 0o640)
	}
	if !fileInfo.ModTime().Equal(fileTime) {
		t.Fatalf("file mod time = %v, want %v", fileInfo.ModTime(), fileTime)
	}

	linkInfo, err := os.Lstat(copiedLink)
	if err != nil {
		t.Fatalf("Lstat(copiedLink) failed: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%q is not a symlink", copiedLink)
	}
	target, err := os.Readlink(copiedLink)
	if err != nil {
		t.Fatalf("Readlink(copiedLink) failed: %v", err)
	}
	if target != filepath.Join("nested", "file.txt") {
		t.Fatalf("symlink target = %q, want %q", target, filepath.Join("nested", "file.txt"))
	}
}

func TestCopyDirRejectsDestinationInsideSourceViaSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := EnsureDir(filepath.Join(src, "nested")); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}

	linkRoot := filepath.Join(root, "link")
	if err := os.Symlink(src, linkRoot); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Symlink requires elevated privileges on Windows: %v", err)
		}
		t.Fatalf("Symlink() failed: %v", err)
	}

	dst := filepath.Join(linkRoot, "nested", "copy")
	err := CopyDir(src, dst)
	if err == nil {
		t.Fatal("CopyDir() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "inside source") {
		t.Fatalf("CopyDir() error = %v, want destination-inside-source error", err)
	}
}

func TestCopyDirPreservesReadOnlyDirectoryPermissions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	lockedDir := filepath.Join(src, "locked")
	filePath := filepath.Join(lockedDir, "file.txt")

	if err := EnsureDir(lockedDir); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := os.Chmod(lockedDir, 0o555); err != nil {
		t.Fatalf("Chmod() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(lockedDir, 0o755)
	})

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir() failed: %v", err)
	}

	copiedDir := filepath.Join(dst, "locked")
	t.Cleanup(func() {
		_ = os.Chmod(copiedDir, 0o755)
	})
	if _, err := os.Stat(filepath.Join(copiedDir, "file.txt")); err != nil {
		t.Fatalf("copied file stat failed: %v", err)
	}

	info, err := os.Stat(copiedDir)
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), 0o555)
	}
}

func TestCopyDirRejectsCaseInsensitiveDestinationInsideSource(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("case-insensitive destination guard is platform-specific")
	}

	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := EnsureDir(filepath.Join(src, "nested")); err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}

	upperSrc := strings.ToUpper(src)
	if _, err := os.Stat(upperSrc); err != nil {
		t.Skipf("filesystem is case-sensitive for temp dir paths: %v", err)
	}

	dst := filepath.Join(upperSrc, "nested", "copy")
	err := CopyDir(src, dst)
	if err == nil {
		t.Fatal("CopyDir() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "inside source") {
		t.Fatalf("CopyDir() error = %v, want destination-inside-source error", err)
	}
}
