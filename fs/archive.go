package fs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// ArchiveEntry is a single file to place into an archive. The archive records only
// the member Name (its path inside the archive, always stored with forward slashes)
// and the Unix Mode; the on-disk Source's timestamps and ownership are deliberately
// not captured, keeping the archive deterministic.
type ArchiveEntry struct {
	// Name is the member name recorded inside the archive (e.g. "toven").
	Name string
	// Source is the path to the on-disk file whose contents are archived.
	Source string
	// Mode is the Unix permission bits recorded for the member (e.g. 0o755).
	Mode uint32
}

// NewArchiveEntry constructs an archive entry from a member name, a source file, and
// a Unix mode.
func NewArchiveEntry(name, source string, mode uint32) ArchiveEntry {
	return ArchiveEntry{Name: name, Source: source, Mode: mode}
}

// zipEpoch is the fixed member modification time for deterministic zip output; the
// zip format's own epoch begins in 1980.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// TarGz packages entries into a deterministic gzip-compressed tar archive at out.
// Each member's tar header pins mtime 0, numeric owner 0:0, and empty owner/group
// names; the gzip wrapper pins mtime 0 and a fixed operating-system byte, so the
// output is a byte-stable function of the entry list (names, contents, modes).
func TarGz(entries []ArchiveEntry, out string) error {
	file, err := createArchiveOut(out)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gz := gzip.NewWriter(file)
	gz.ModTime = time.Time{}
	gz.OS = 255
	tw := tar.NewWriter(gz)

	for _, entry := range entries {
		if err := appendTarEntry(tw, entry, out); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return archiveIOError(out, "finish tar stream", err)
	}
	if err := gz.Close(); err != nil {
		return archiveIOError(out, "finish gzip stream", err)
	}
	if err := file.Sync(); err != nil {
		return archiveIOError(out, "flush archive", err)
	}
	return nil
}

func appendTarEntry(tw *tar.Writer, entry ArchiveEntry, out string) error {
	source, size, err := openArchiveSource(entry.Source)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     entry.Name,
		Size:     size,
		Mode:     int64(entry.Mode),
		ModTime:  time.Unix(0, 0).UTC(),
		Format:   tar.FormatGNU,
	}
	if err := tw.WriteHeader(header); err != nil {
		return archiveIOError(out, "append tar member", err)
	}
	if _, err := io.Copy(tw, source); err != nil {
		return archiveIOError(out, "append tar member", err)
	}
	return nil
}

// Zip packages entries into a deterministic zip archive at out. Each member uses the
// DEFLATE method, the fixed zip epoch (1980-01-01) as its modification time, and the
// caller-supplied Unix mode; nothing host-derived is recorded, so identical inputs
// produce byte-identical output.
func Zip(entries []ArchiveEntry, out string) error {
	file, err := createArchiveOut(out)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	zw := zip.NewWriter(file)
	for _, entry := range entries {
		if err := appendZipEntry(zw, entry, out); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return archiveIOError(out, "finish zip archive", err)
	}
	if err := file.Sync(); err != nil {
		return archiveIOError(out, "flush archive", err)
	}
	return nil
}

func appendZipEntry(zw *zip.Writer, entry ArchiveEntry, out string) error {
	source, _, err := openArchiveSource(entry.Source)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	header := &zip.FileHeader{
		Name:     entry.Name,
		Method:   zip.Deflate,
		Modified: zipEpoch,
	}
	header.SetMode(os.FileMode(entry.Mode).Perm())
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return archiveIOError(out, "start zip member", err)
	}
	if _, err := io.Copy(writer, source); err != nil {
		return archiveIOError(out, "write zip member", err)
	}
	return nil
}

func createArchiveOut(out string) (*os.File, error) {
	file, err := os.Create(out)
	if err != nil {
		code, status := osErrorCode(err)
		return nil, apperrors.New(code,
			fmt.Sprintf("cannot create archive '%s': %v", out, err), status).WithCause(err)
	}
	return file, nil
}

func openArchiveSource(source string) (*os.File, int64, error) {
	file, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, apperrors.InvalidInput("source",
				fmt.Sprintf("archive source '%s' does not exist", source)).WithCause(err)
		}
		code, status := osErrorCode(err)
		return nil, 0, apperrors.New(code,
			fmt.Sprintf("cannot open archive source '%s': %v", source, err), status).WithCause(err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		code, status := osErrorCode(err)
		return nil, 0, apperrors.New(code,
			fmt.Sprintf("cannot stat archive source '%s': %v", source, err), status).WithCause(err)
	}
	return file, info.Size(), nil
}

func archiveIOError(out, action string, err error) error {
	return apperrors.New(apperrors.ErrCodeInternal,
		fmt.Sprintf("cannot %s for '%s': %v", action, out, err), 500).WithCause(err)
}
