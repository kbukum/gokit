package fs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
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
	writeMaliciousTarGz(t, archivePath, "../escape.txt")
	if _, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits()); err == nil {
		t.Fatal("expected tar-slip rejection")
	}
}

func TestExtractTarGzRejectsSymlink(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "link.tar.gz")
	writeSymlinkTarGz(t, archivePath)
	if _, err := ExtractTarGz(archivePath, t.TempDir(), DefaultExtractLimits()); err == nil {
		t.Fatal("expected symlink rejection")
	}
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

func writeMaliciousTarGz(t *testing.T, archivePath, memberName string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	content := []byte("pwn")
	if err := tw.WriteHeader(&tar.Header{
		Name: memberName, Size: int64(len(content)), Mode: 0o644, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeSymlinkTarGz(t *testing.T, archivePath string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink, Mode: 0o777,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
