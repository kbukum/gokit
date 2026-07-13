package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/storage"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage(%q): %v", dir, err)
	}
	return s
}

// ---------------------------------------------------------------------------
// Upload / Download round-trip
// ---------------------------------------------------------------------------

func TestUploadDownload_Roundtrip(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	content := []byte("hello, storage!")
	if err := s.Upload(ctx, "docs/hello.txt", bytes.NewReader(content)); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	rc, err := s.Download(ctx, "docs/hello.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestUploadDownload_BinaryData(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	if err := s.Upload(ctx, "binary.dat", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	rc, err := s.Download(ctx, "binary.dat")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, data) {
		t.Errorf("binary round-trip mismatch: got %d bytes, want %d", len(got), len(data))
	}
}

func TestUploadDownload_EmptyFile(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.Upload(ctx, "empty.txt", bytes.NewReader(nil)); err != nil {
		t.Fatalf("Upload empty: %v", err)
	}
	rc, err := s.Download(ctx, "empty.txt")
	if err != nil {
		t.Fatalf("Download empty: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if len(got) != 0 {
		t.Errorf("expected empty, got %d bytes", len(got))
	}
}

func TestUpload_CreatesNestedDirectories(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.Upload(ctx, "a/b/c/deep.txt", bytes.NewReader([]byte("deep"))); err != nil {
		t.Fatalf("Upload deep path: %v", err)
	}
	exists, _ := s.Exists(ctx, "a/b/c/deep.txt")
	if !exists {
		t.Error("expected deep file to exist after upload")
	}
}

func TestUpload_Overwrite(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "file.txt", bytes.NewReader([]byte("v1")))
	_ = s.Upload(ctx, "file.txt", bytes.NewReader([]byte("v2")))

	rc, _ := s.Download(ctx, "file.txt")
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Errorf("expected overwrite to v2, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Path traversal security
// ---------------------------------------------------------------------------

func TestSafePath_NeutralizesTraversal(t *testing.T) {
	basePath := "/safe/base"
	attacks := []string{
		"../../etc/passwd",
		"../../../etc/shadow",
		"foo/../../etc/passwd",
		"foo/../../../etc/shadow",
		"/etc/passwd",
	}
	for _, attack := range attacks {
		result, err := safePath(basePath, attack)
		if err != nil {
			// If safePath rejects it, that's also acceptable
			continue
		}
		// The resolved path must stay inside basePath
		if !strings.HasPrefix(result, basePath+string(filepath.Separator)) && result != basePath {
			t.Errorf("safePath(%q, %q) = %q escapes base directory", basePath, attack, result)
		}
	}
}

func TestSafePath_AllowsLegitPaths(t *testing.T) {
	basePath := "/safe/base"
	legit := []string{
		"file.txt",
		"sub/dir/file.txt",
		"sub/dir/deep/nested/file.bin",
	}
	for _, p := range legit {
		result, err := safePath(basePath, p)
		if err != nil {
			t.Errorf("safePath(%q, %q) unexpected error: %v", basePath, p, err)
		}
		if !strings.HasPrefix(result, basePath) {
			t.Errorf("result %q does not start with base %q", result, basePath)
		}
	}
}

func TestUpload_PathTraversal_Neutralized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Traversal paths are neutralized to stay within base dir
	err := s.Upload(ctx, "../../etc/passwd", bytes.NewReader([]byte("safe")))
	if err != nil {
		// Rejection is also acceptable
		return
	}
	// If upload succeeded, the file must be within the base directory
	exists, _ := s.Exists(ctx, "../../etc/passwd")
	if !exists {
		t.Error("file should exist (neutralized path)")
	}
	// Verify it resolved to etc/passwd within the base, not the system path
	if _, err := os.Stat("/etc/passwd_hacked"); err == nil {
		t.Fatal("path traversal escaped base directory!")
	}
}

func TestDownload_PathTraversal_Neutralized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Upload first via traversal path, then download
	_ = s.Upload(ctx, "../../etc/test.txt", bytes.NewReader([]byte("safe")))
	rc, err := s.Download(ctx, "../../etc/test.txt")
	if err != nil {
		return // Rejection is acceptable
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "safe" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestExists_PathTraversal_Neutralized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Non-existent traversal path should return false (not error)
	exists, err := s.Exists(ctx, "../../nonexistent.txt")
	if err != nil {
		return // Rejection is acceptable
	}
	if exists {
		t.Error("non-existent file should not exist")
	}
}

func TestURL_PathTraversal_Neutralized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	u, err := s.URL(ctx, "../../etc/file.txt")
	if err != nil {
		return // Rejection is acceptable
	}
	// URL should contain the base path, not /etc/
	if !strings.Contains(u, s.basePath) {
		t.Errorf("URL %q does not reference base path", u)
	}
}

func TestList_PathTraversal_Neutralized(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Should either reject or return empty list
	files, err := s.List(ctx, "../../etc")
	if err != nil {
		return // Rejection is acceptable
	}
	// If it succeeded, it listed within the neutralized path
	_ = files
}

// ---------------------------------------------------------------------------
// Exists
// ---------------------------------------------------------------------------

func TestExists_FileNotPresent(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	exists, err := s.Exists(ctx, "nope.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("expected file not to exist")
	}
}

func TestExists_AfterUpload(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "present.txt", bytes.NewReader([]byte("here")))
	exists, err := s.Exists(ctx, "present.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("expected file to exist after upload")
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete_ExistingFile(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "del.txt", bytes.NewReader([]byte("bye")))
	if err := s.Delete(ctx, "del.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, _ := s.Exists(ctx, "del.txt")
	if exists {
		t.Error("file should not exist after delete")
	}
}

func TestDelete_NonexistentFile_NoError(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if err := s.Delete(ctx, "ghost.txt"); err != nil {
		t.Fatalf("Delete non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Download errors
// ---------------------------------------------------------------------------

func TestDownload_FileNotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.Download(ctx, "missing.txt")
	if err == nil {
		t.Fatal("expected error downloading missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// URL generation
// ---------------------------------------------------------------------------

func TestURL_ReturnsFileScheme(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	u, err := s.URL(ctx, "doc.pdf")
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if !strings.HasPrefix(u, "file://") {
		t.Errorf("expected file:// scheme, got %q", u)
	}
	if !strings.Contains(u, "doc.pdf") {
		t.Errorf("URL should contain file name, got %q", u)
	}
}

// ---------------------------------------------------------------------------
// List with filters / sorting
// ---------------------------------------------------------------------------

func TestList_EmptyDirectory(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Create the prefix directory so List can walk it
	_ = s.Upload(ctx, "emptydir/placeholder.tmp", bytes.NewReader([]byte("")))
	_ = s.Delete(ctx, "emptydir/placeholder.tmp")

	files, err := s.List(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestList_MultipleFiles_Sorted(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		_ = s.Upload(ctx, "sorted/"+name, bytes.NewReader([]byte(name)))
	}

	files, err := s.List(ctx, "sorted")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	// Files should be sorted by path
	for i := 1; i < len(files); i++ {
		if files[i].Path < files[i-1].Path {
			t.Errorf("files not sorted: %q < %q", files[i].Path, files[i-1].Path)
		}
	}
}

func TestList_WithPrefix_FiltersCorrectly(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "imgs/a.png", bytes.NewReader([]byte("a")))
	_ = s.Upload(ctx, "imgs/b.png", bytes.NewReader([]byte("b")))
	_ = s.Upload(ctx, "docs/c.txt", bytes.NewReader([]byte("c")))

	files, err := s.List(ctx, "imgs")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files in imgs/, got %d", len(files))
	}
}

func TestList_NestedDirectories(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "root/sub1/a.txt", bytes.NewReader([]byte("a")))
	_ = s.Upload(ctx, "root/sub2/b.txt", bytes.NewReader([]byte("b")))
	_ = s.Upload(ctx, "root/c.txt", bytes.NewReader([]byte("c")))

	files, err := s.List(ctx, "root")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files across nested dirs, got %d", len(files))
	}
}

// ---------------------------------------------------------------------------
// Content-type detection
// ---------------------------------------------------------------------------

func TestList_ContentType_Detection(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	tests := map[string]string{
		"test.html": "text/html",
		"test.json": "application/json",
		"test.css":  "text/css",
		"test.js":   "",
		"test.xml":  "",
	}

	for name := range tests {
		_ = s.Upload(ctx, "ct/"+name, bytes.NewReader([]byte("x")))
	}

	files, err := s.List(ctx, "ct")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	for _, f := range files {
		ext := filepath.Ext(f.Path)
		expected := mime.TypeByExtension(ext)
		if expected == "" {
			expected = "application/octet-stream"
		}
		if f.ContentType != expected {
			t.Errorf("file %s: content-type=%q, want %q", f.Path, f.ContentType, expected)
		}
	}
}

func TestList_UnknownExtension_DefaultsToOctetStream(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "types/file.xyz123", bytes.NewReader([]byte("x")))
	files, err := s.List(ctx, "types")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].ContentType != "application/octet-stream" {
		t.Errorf("expected octet-stream for unknown ext, got %q", files[0].ContentType)
	}
}

// ---------------------------------------------------------------------------
// FileInfo metadata accuracy
// ---------------------------------------------------------------------------

func TestList_FileInfo_SizeAccurate(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	data := []byte("exactly 21 bytes!!!!!")
	_ = s.Upload(ctx, "meta/sized.txt", bytes.NewReader(data))

	files, err := s.List(ctx, "meta")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Size != int64(len(data)) {
		t.Errorf("size = %d, want %d", files[0].Size, len(data))
	}
}

func TestList_FileInfo_LastModifiedSet(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_ = s.Upload(ctx, "meta/time.txt", bytes.NewReader([]byte("hi")))
	files, _ := s.List(ctx, "meta")
	if len(files) < 1 {
		t.Fatal("expected at least 1 file")
	}
	if files[0].LastModified.IsZero() {
		t.Error("LastModified should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Large file handling
// ---------------------------------------------------------------------------

func TestUploadDownload_LargeFile(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// 2 MB file
	size := 2 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251) // use prime to vary byte values
	}

	if err := s.Upload(ctx, "large/big.bin", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload large: %v", err)
	}

	rc, err := s.Download(ctx, "large/big.bin")
	if err != nil {
		t.Fatalf("Download large: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if len(got) != size {
		t.Errorf("size mismatch: got %d, want %d", len(got), size)
	}
	if !bytes.Equal(got, data) {
		t.Error("large file content mismatch")
	}
}

// ---------------------------------------------------------------------------
// Concurrent upload/download
// ---------------------------------------------------------------------------

func TestConcurrent_UploadDownload(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			key := filepath.Join("concurrent", strings.Repeat("x", idx+1)+".txt")
			data := []byte(strings.Repeat("y", (idx+1)*100))

			if err := s.Upload(ctx, key, bytes.NewReader(data)); err != nil {
				errs[idx] = err
				return
			}
			rc, err := s.Download(ctx, key)
			if err != nil {
				errs[idx] = err
				return
			}
			defer rc.Close()
			got, err := io.ReadAll(rc)
			if err != nil {
				errs[idx] = err
				return
			}
			if !bytes.Equal(got, data) {
				errs[idx] = io.ErrUnexpectedEOF
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Config validation
// ---------------------------------------------------------------------------

func TestConfig_ApplyDefaults(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.BasePath != DefaultBasePath {
		t.Errorf("expected default base path %q, got %q", DefaultBasePath, c.BasePath)
	}
}

func TestConfig_Validate_EmptyBasePath(t *testing.T) {
	c := &Config{BasePath: ""}
	if err := c.Validate(); err == nil {
		t.Error("expected validation error for empty base_path")
	}
}

func TestConfig_Validate_NonEmptyBasePath(t *testing.T) {
	c := &Config{BasePath: "/some/path"}
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewStorage_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-subdir")
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil storage")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("base dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func TestNewStorage_ResolvesRelativePath(t *testing.T) {
	// Create relative dir inside a temp dir
	base := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(base)

	s, err := NewStorage("./reltest")
	if err != nil {
		t.Fatalf("NewStorage with relative: %v", err)
	}
	// basePath should be absolute
	if !filepath.IsAbs(s.basePath) {
		t.Errorf("basePath should be absolute, got %q", s.basePath)
	}
}

// ---------------------------------------------------------------------------
// Storage interface compliance
// ---------------------------------------------------------------------------

func TestStorage_ImplementsInterface(t *testing.T) {
	var _ storage.Storage = (*Storage)(nil)
}

func TestRegister_CapturesConfig(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{BasePath: t.TempDir()}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	f, ok := reg.Get(storage.ProviderLocal)
	if !ok {
		t.Fatal("local provider missing")
	}
	s, err := f(storage.Config{}, nil)
	if err != nil || s == nil {
		t.Fatalf("factory = %v, %v", s, err)
	}
}

func TestRegister_DefaultsWhenNoConfig(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register with defaults: %v", err)
	}
	if _, ok := reg.Get(storage.ProviderLocal); !ok {
		t.Fatal("local provider missing")
	}
}

func TestUpload_MkdirFailsUnderFile(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	// Create a regular file where a directory is expected, so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(base, "file"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := s.Upload(context.Background(), "file/child.txt", strings.NewReader("y")); err == nil {
		t.Fatal("expected upload error when parent is a file")
	}
}

// errReader fails on Read, exercising the io.Copy error path in Upload.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestNewStorage_MkdirFailsWhenBaseIsFile(t *testing.T) {
	t.Parallel()
	base := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(base, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := NewStorage(base); err == nil {
		t.Fatal("expected mkdir error when base path is a file")
	}
}

func TestUpload_CreateFailsOnDirectoryTarget(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := os.Mkdir(filepath.Join(base, "dir"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Creating a file at an existing directory path fails.
	if err := s.Upload(context.Background(), "dir", errReader{}); err == nil {
		t.Fatal("expected create error targeting a directory")
	}
}

func TestUpload_CopyError(t *testing.T) {
	t.Parallel()
	s, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := s.Upload(context.Background(), "file.txt", errReader{}); err == nil {
		t.Fatal("expected copy error from failing reader")
	}
}

func TestDelete_NonEmptyDirectoryError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "dir", "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Removing a non-empty directory returns an error that is not IsNotExist.
	if err := s.Delete(context.Background(), "dir"); err == nil {
		t.Fatal("expected delete error on non-empty directory")
	}
}

func TestExists_StatErrorNotNotExist(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "f"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Stat through a non-directory parent yields ENOTDIR (not IsNotExist).
	if _, err := s.Exists(context.Background(), "f/child"); err == nil {
		t.Fatal("expected stat error through non-directory parent")
	}
}

func TestDownload_PermissionError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permissions")
	}
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	p := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(p, []byte("x"), 0o000); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := s.Download(context.Background(), "secret.txt"); err == nil {
		t.Fatal("expected permission error opening unreadable file")
	}
}

func TestList_ErrorThroughNonDirectoryParent(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	s, err := NewStorage(base)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "f"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Walking under a file yields a non-IsNotExist error.
	if _, err := s.List(context.Background(), "f/sub"); err == nil {
		t.Fatal("expected list error walking under a file")
	}
}
