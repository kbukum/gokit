package testutil

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"testing"
)

// TarGzEntry describes one member in a gzip-compressed tar fixture.
type TarGzEntry struct {
	Name     string
	Body     []byte
	Typeflag byte
	Linkname string
	Mode     int64
}

// TarFile returns a regular-file tar member fixture.
func TarFile(name, body string) TarGzEntry {
	return TarGzEntry{Name: name, Body: []byte(body), Typeflag: tar.TypeReg, Mode: 0o644}
}

// TarSymlink returns a symlink tar member fixture.
func TarSymlink(name, target string) TarGzEntry {
	return TarGzEntry{Name: name, Linkname: target, Typeflag: tar.TypeSymlink, Mode: 0o777}
}

// TarDir returns a directory tar member fixture.
func TarDir(name string) TarGzEntry {
	return TarGzEntry{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}
}

// WriteTarGz writes entries as a gzip-compressed tar archive at archivePath.
func WriteTarGz(t *testing.T, archivePath string, entries ...TarGzEntry) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		if entry.Mode == 0 {
			entry.Mode = 0o644
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     entry.Name,
			Size:     int64(len(entry.Body)),
			Mode:     entry.Mode,
			Typeflag: entry.Typeflag,
			Linkname: entry.Linkname,
		}); err != nil {
			t.Fatal(err)
		}
		if len(entry.Body) > 0 {
			if _, err := tw.Write(entry.Body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

// ZipEntry describes one member in a zip fixture.
type ZipEntry struct {
	Name string
	Body []byte
	Mode os.FileMode
}

// ZipFile returns a regular-file zip member fixture.
func ZipFile(name, body string) ZipEntry {
	return ZipEntry{Name: name, Body: []byte(body), Mode: 0o644}
}

// ZipSymlink returns a symlink zip member fixture.
func ZipSymlink(name, target string) ZipEntry {
	return ZipEntry{Name: name, Body: []byte(target), Mode: os.ModeSymlink | 0o777}
}

// WriteZip writes entries as a zip archive at archivePath.
func WriteZip(t *testing.T, archivePath string, entries ...ZipEntry) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	zw := zip.NewWriter(file)
	for _, entry := range entries {
		if entry.Mode == 0 {
			entry.Mode = 0o644
		}
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		header.SetMode(entry.Mode)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if len(entry.Body) > 0 {
			if _, err := writer.Write(entry.Body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
