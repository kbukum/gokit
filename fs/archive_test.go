package fs

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	fstestutil "github.com/kbukum/gokit/fs/testutil"
)

func writeSource(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTarGzRoundTrip(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "alpha")
	b := writeSource(t, src, "b.txt", "beta")
	archivePath := filepath.Join(t.TempDir(), "out.tar.gz")
	entries := []ArchiveEntry{
		NewArchiveEntry("a.txt", a, 0o644),
		NewArchiveEntry("nested/b.txt", b, 0o600),
	}
	if err := TarGz(entries, archivePath); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	extracted, err := ExtractTarGz(archivePath, dest, DefaultExtractLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(extracted) != 2 {
		t.Fatalf("extracted %d entries", len(extracted))
	}
	got, err := os.ReadFile(filepath.Join(dest, "nested", "b.txt"))
	if err != nil || string(got) != "beta" {
		t.Fatalf("extracted content = %q, %v", got, err)
	}
}

func TestTarGzDeterministic(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "alpha")
	entries := []ArchiveEntry{NewArchiveEntry("a.txt", a, 0o644)}

	first := filepath.Join(t.TempDir(), "first.tar.gz")
	second := filepath.Join(t.TempDir(), "second.tar.gz")
	if err := TarGz(entries, first); err != nil {
		t.Fatal(err)
	}
	if err := TarGz(entries, second); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(first)
	secondBytes, _ := os.ReadFile(second)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("tar.gz output is not byte-stable")
	}
}

func TestZipRoundTrip(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "alpha")
	archivePath := filepath.Join(t.TempDir(), "out.zip")
	if err := Zip([]ArchiveEntry{NewArchiveEntry("a.txt", a, 0o644)}, archivePath); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := ExtractZip(archivePath, dest, DefaultExtractLimits()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "a.txt"))
	if err != nil || string(got) != "alpha" {
		t.Fatalf("extracted content = %q, %v", got, err)
	}
}

func TestZipDeterministic(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "alpha")
	entries := []ArchiveEntry{NewArchiveEntry("a.txt", a, 0o644)}
	first := filepath.Join(t.TempDir(), "first.zip")
	second := filepath.Join(t.TempDir(), "second.zip")
	if err := Zip(entries, first); err != nil {
		t.Fatal(err)
	}
	if err := Zip(entries, second); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(first)
	secondBytes, _ := os.ReadFile(second)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("zip output is not byte-stable")
	}
}

func TestTarGzMissingSourceErrors(t *testing.T) {
	t.Parallel()
	entries := []ArchiveEntry{NewArchiveEntry("a.txt", filepath.Join(t.TempDir(), "missing"), 0o644)}
	if err := TarGz(entries, filepath.Join(t.TempDir(), "out.tar.gz")); err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestExtractTarGzRejectsSlip(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "evil.tar.gz")
	fstestutil.WriteTarGz(t, archivePath, fstestutil.TarFile("../escape.txt", "pwn"))
	if _, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits()); err == nil {
		t.Fatal("expected tar-slip rejection")
	}
}

func TestExtractTarGzRejectsSymlink(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "link.tar.gz")
	fstestutil.WriteTarGz(t, archivePath, fstestutil.TarSymlink("link", "/etc/passwd"))
	if _, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits()); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestExtractTarGzRejectsHardLink(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "link.tar.gz")
	fstestutil.WriteTarGz(t, archivePath, fstestutil.TarGzEntry{
		Name: "link", Typeflag: tar.TypeLink, Linkname: "../outside",
	})
	_, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits())
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
}

func TestExtractTarGzCreatesDirectoryMember(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "dir.tar.gz")
	fstestutil.WriteTarGz(t, archivePath, fstestutil.TarDir("nested/"))

	dest := t.TempDir()
	if _, err := ExtractTarGz(archivePath, dest, DefaultExtractLimits()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dest, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected extracted member to be a directory")
	}
}

func TestExtractTarGzMalformedInputReturnsTypedInvalidInput(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "short.tar.gz")
	if err := os.WriteFile(archivePath, []byte{0x1f, 0x8b}, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits())
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected the underlying gzip cause to be preserved, got %v", err)
	}
}

func TestExtractZipMalformedInputReturnsTypedInvalidInput(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "malformed.zip")
	if err := os.WriteFile(archivePath, []byte("not a zip archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractZip(archivePath, t.TempDir(), DefaultExtractLimits())
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
	if !errors.Is(err, zip.ErrFormat) {
		t.Fatalf("expected the underlying zip cause to be preserved, got %v", err)
	}
}

func TestExtractZipRejectsSlip(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "evil.zip")
	fstestutil.WriteZip(t, archivePath, fstestutil.ZipFile("../escape.txt", "pwn"))

	_, err := ExtractZip(archivePath, t.TempDir(), DefaultExtractLimits())
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
}

func TestExtractZipRejectsSymlink(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "link.zip")
	fstestutil.WriteZip(t, archivePath, fstestutil.ZipSymlink("link", "/etc/passwd"))

	_, err := ExtractZip(archivePath, t.TempDir(), DefaultExtractLimits())
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
}

func TestExtractZipEnforcesByteLimit(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "big.zip")
	fstestutil.WriteZip(t, archivePath, fstestutil.ZipFile("big.txt", "0123456789"))
	limits := DefaultExtractLimits().WithMaxTotalBytes(4)

	_, err := ExtractZip(archivePath, t.TempDir(), limits)
	assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
}

func TestExtractTarGzEnforcesEntryLimit(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "alpha")
	archivePath := filepath.Join(t.TempDir(), "many.tar.gz")
	if err := TarGz([]ArchiveEntry{
		NewArchiveEntry("a.txt", a, 0o644),
		NewArchiveEntry("b.txt", a, 0o644),
	}, archivePath); err != nil {
		t.Fatal(err)
	}
	limits := DefaultExtractLimits().WithMaxEntries(1)
	if _, err := ExtractTarGz(archivePath, t.TempDir(), limits); err == nil {
		t.Fatal("expected entry-limit rejection")
	}
}

func TestExtractTarGzEnforcesByteLimit(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	a := writeSource(t, src, "a.txt", "0123456789")
	archivePath := filepath.Join(t.TempDir(), "big.tar.gz")
	if err := TarGz([]ArchiveEntry{NewArchiveEntry("a.txt", a, 0o644)}, archivePath); err != nil {
		t.Fatal(err)
	}
	limits := DefaultExtractLimits().WithMaxTotalBytes(4)
	if _, err := ExtractTarGz(archivePath, t.TempDir(), limits); err == nil {
		t.Fatal("expected byte-limit rejection")
	}
}

func TestExtractTarGzRejectsPreexistingLeafSymlink(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	payload := writeSource(t, src, "payload", "pwn")
	archivePath := filepath.Join(t.TempDir(), "leaf.tar.gz")
	if err := TarGz([]ArchiveEntry{NewArchiveEntry("victim", payload, 0o644)}, archivePath); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "victim")); err != nil {
		t.Fatal(err)
	}

	if _, err := ExtractTarGz(archivePath, dest, DefaultExtractLimits()); err == nil {
		t.Fatal("expected rejection of a pre-existing leaf symlink in dest")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("extraction wrote through the leaf symlink; outside file = %q", got)
	}
}

func assertAppErrorCode(t *testing.T, err error, want goerrors.ErrorCode) {
	t.Helper()
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("error = %T, want *errors.AppError", err)
	}
	if appErr.Code != want {
		t.Fatalf("code = %s, want %s", appErr.Code, want)
	}
}

// TestSanitizeArchivePath verifies the component-aware containment guard admits
// legitimate destinations (including a root ending in a separator and a member
// resolving to the destination itself) while still rejecting sibling-prefix and
// traversal escapes.
func TestSanitizeArchivePath(t *testing.T) {
	sep := string(filepath.Separator)
	root := t.TempDir()

	tests := []struct {
		name    string
		dest    string
		member  string
		wantErr bool
	}{
		{"plain file", root, "file.txt", false},
		{"nested file", root, filepath.Join("a", "b.txt"), false},
		{"member equals root", root, ".", false},
		{"dest with trailing separator", root + sep, "file.txt", false},
		{"traversal escape", root, filepath.Join("..", "evil"), true},
		{"absolute escape", root, sep + "etc", true},
		{"legitimate dots in filename", root, "file..txt", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sanitizeArchivePath(tc.dest, tc.member)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for member %q under %q, got %q", tc.member, tc.dest, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for member %q under %q: %v", tc.member, tc.dest, err)
			}
		})
	}
}

// TestSanitizeArchivePathRejectsSiblingPrefix guards the "/out" vs "/output"
// sibling-prefix case that a naive string-prefix check would let through.
func TestSanitizeArchivePathRejectsSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "out")
	// A member crafted to resolve into a sibling directory sharing the prefix.
	if _, err := sanitizeArchivePath(root, filepath.Join("..", "output", "x")); err == nil {
		t.Fatal("expected sibling-prefix escape to be rejected")
	}
}

func TestArchiveExtractionPathBoundary(t *testing.T) {
	t.Parallel()
	formats := []struct {
		name    string
		write   func(*testing.T, string, string)
		extract func(string, string, ExtractLimits) ([]string, error)
	}{
		{"tar", func(t *testing.T, path, member string) {
			fstestutil.WriteTarGz(t, path, fstestutil.TarFile(member, "replacement"))
		}, ExtractTarGz},
		{"zip", func(t *testing.T, path, member string) {
			fstestutil.WriteZip(t, path, fstestutil.ZipFile(member, "replacement"))
		}, ExtractZip},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			t.Parallel()
			for _, scenario := range []string{
				"traversal", "nested traversal", "sibling prefix", "absolute", "drive prefix",
				"parent symlink", "leaf symlink", "nested file", "dots in filename",
			} {
				t.Run(scenario, func(t *testing.T) {
					t.Parallel()
					base := t.TempDir()
					dest := filepath.Join(base, "out")
					outside := filepath.Join(base, "output")
					if err := os.MkdirAll(outside, 0o750); err != nil {
						t.Fatal(err)
					}
					victim := filepath.Join(outside, "victim")
					if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
						t.Fatal(err)
					}
					before, err := os.Stat(victim)
					if err != nil {
						t.Fatal(err)
					}
					member := "nested/file.txt"
					wantErr := true
					switch scenario {
					case "traversal":
						member = "../output/victim"
					case "nested traversal":
						member = "nested/../../output/victim"
					case "sibling prefix":
						member = "../output/new/victim"
					case "absolute":
						member = filepath.ToSlash(victim)
					case "drive prefix":
						member = "C:/output/victim"
					case "parent symlink", "leaf symlink":
						if err := os.MkdirAll(dest, 0o750); err != nil {
							t.Fatal(err)
						}
						linkTarget := outside
						member = "link/victim"
						if scenario == "leaf symlink" {
							linkTarget = victim
							member = "link"
						}
						if err := os.Symlink(linkTarget, filepath.Join(dest, "link")); err != nil {
							t.Fatal(err)
						}
					case "nested file":
						wantErr = false
					case "dots in filename":
						member = "nested/file..txt"
						wantErr = false
					}
					archive := filepath.Join(base, "archive")
					format.write(t, archive, member)
					paths, err := format.extract(archive, dest+string(filepath.Separator), DefaultExtractLimits())
					if wantErr {
						assertAppErrorCode(t, err, goerrors.ErrCodeInvalidInput)
					} else {
						if err != nil {
							t.Fatal(err)
						}
						want := filepath.Join(dest, member)
						if len(paths) != 1 || paths[0] != want {
							t.Fatalf("extracted paths = %v, want [%s]", paths, want)
						}
						got, err := os.ReadFile(want)
						if err != nil || string(got) != "replacement" {
							t.Fatalf("extracted content = %q, %v", got, err)
						}
					}
					got, err := os.ReadFile(victim)
					if err != nil || string(got) != "original" {
						t.Fatalf("outside content changed: %q, %v", got, err)
					}
					after, err := os.Stat(victim)
					if err != nil || after.Mode() != before.Mode() {
						t.Fatalf("outside permissions changed: %v", err)
					}
					entries, err := os.ReadDir(outside)
					if err != nil || len(entries) != 1 || entries[0].Name() != "victim" {
						t.Fatalf("outside directory changed: %v, %v", entries, err)
					}
				})
			}
		})
	}
}
