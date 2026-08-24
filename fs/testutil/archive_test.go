package testutil

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTarGz(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "fixture.tar.gz")
	WriteTarGz(t, archivePath, TarDir("dir"), TarFile("dir/file.txt", "body"), TarSymlink("link", "dir/file.txt"))

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)

	var names []string
	for {
		header, err := reader.Next()
		if err != nil {
			break
		}
		names = append(names, header.Name)
	}
	if len(names) != 3 {
		t.Fatalf("names = %v, want 3 entries", names)
	}
}

func TestWriteZip(t *testing.T) {
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "fixture.zip")
	WriteZip(t, archivePath, ZipFile("file.txt", "body"), ZipSymlink("link", "file.txt"))

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if len(reader.File) != 2 {
		t.Fatalf("zip entries = %d, want 2", len(reader.File))
	}
	if reader.File[1].Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected second zip member to be marked as symlink")
	}
}
