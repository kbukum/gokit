package fs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	// DefaultMaxTotalBytes is the default cap on total uncompressed output (512 MiB).
	DefaultMaxTotalBytes int64 = 512 * 1024 * 1024
	// DefaultMaxEntries is the default cap on the number of archive members.
	DefaultMaxEntries int = 4096
)

// ExtractLimits bounds an extraction of an untrusted archive. Extraction reads
// attacker-controlled input, so both the number of members and the total number of
// uncompressed bytes written are capped to defuse decompression bombs. Exceeding
// either bound fails the whole extraction closed.
type ExtractLimits struct {
	// MaxTotalBytes is the maximum total uncompressed bytes written across all members.
	MaxTotalBytes int64
	// MaxEntries is the maximum number of members processed.
	MaxEntries int
}

// DefaultExtractLimits returns the default extraction bounds.
func DefaultExtractLimits() ExtractLimits {
	return ExtractLimits{MaxTotalBytes: DefaultMaxTotalBytes, MaxEntries: DefaultMaxEntries}
}

// WithMaxTotalBytes returns a copy of the limits with MaxTotalBytes set.
func (l ExtractLimits) WithMaxTotalBytes(maxTotalBytes int64) ExtractLimits {
	l.MaxTotalBytes = maxTotalBytes
	return l
}

// WithMaxEntries returns a copy of the limits with MaxEntries set.
func (l ExtractLimits) WithMaxEntries(maxEntries int) ExtractLimits {
	l.MaxEntries = maxEntries
	return l
}

// ExtractTarGz extracts every member of the gzip-compressed tar archive at archivePath
// into dest, returning the extracted file paths in archive order. Extraction is
// hardened against hostile archives: a member whose path is absolute or escapes dest
// via ".." is rejected (tar-slip), symlink and hard-link members are rejected, each
// materialized path is re-checked to resolve within dest so a pre-existing symlink in
// the destination tree cannot redirect a write outside it, and the member count and
// total uncompressed size are bounded by limits. Recorded Unix modes are re-applied.
func ExtractTarGz(archivePath, dest string, limits ExtractLimits) ([]string, error) {
	file, err := openArchive(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, extractIOError(archivePath, "open gzip stream", err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)

	root, err := extractionRoot(archivePath, dest)
	if err != nil {
		return nil, err
	}

	var extracted []string
	var written int64
	index := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, extractIOError(archivePath, "read tar member", err)
		}
		if index >= limits.MaxEntries {
			return nil, tooManyEntriesError(archivePath, limits.MaxEntries)
		}
		index++

		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return nil, unsafeLinkError(archivePath, header.Name)
		}
		target, isDir, skip, err := memberTarget(archivePath, dest, header.Name)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		mode := os.FileMode(header.Mode).Perm()
		if header.Typeflag == tar.TypeDir || isDir {
			if err := extractDir(archivePath, root, target, header.Name, mode); err != nil {
				return nil, err
			}
			continue
		}
		if err := extractFile(archivePath, root, target, header.Name, reader, limits, &written, mode); err != nil {
			return nil, err
		}
		extracted = append(extracted, target)
	}
	return extracted, nil
}

// ExtractZip extracts every member of the zip archive at archivePath into dest,
// returning the extracted file paths in archive order. It is hardened identically to
// ExtractTarGz: escaping members are rejected (zip-slip), symlink members are
// rejected, each materialized path is re-checked to resolve within dest, and limits
// bound the member count and total uncompressed size. Recorded Unix modes are
// re-applied.
func ExtractZip(archivePath, dest string, limits ExtractLimits) ([]string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.InvalidInput("archive",
				fmt.Sprintf("cannot open archive '%s': %v", archivePath, err)).WithCause(err)
		}
		return nil, extractIOError(archivePath, "open zip archive", err)
	}
	defer func() { _ = reader.Close() }()

	if len(reader.File) > limits.MaxEntries {
		return nil, tooManyEntriesError(archivePath, limits.MaxEntries)
	}
	root, err := extractionRoot(archivePath, dest)
	if err != nil {
		return nil, err
	}

	var extracted []string
	var written int64
	for _, member := range reader.File {
		if member.Mode()&os.ModeSymlink != 0 {
			return nil, unsafeLinkError(archivePath, member.Name)
		}
		target, isDir, skip, err := memberTarget(archivePath, dest, member.Name)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		mode := member.Mode().Perm()
		if member.FileInfo().IsDir() || isDir {
			if e := extractDir(archivePath, root, target, member.Name, mode); e != nil {
				return nil, e
			}
			continue
		}
		rc, err := member.Open()
		if err != nil {
			return nil, extractIOError(archivePath, "read zip member", err)
		}
		err = extractFile(archivePath, root, target, member.Name, rc, limits, &written, mode)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		extracted = append(extracted, target)
	}
	return extracted, nil
}

// memberTarget resolves a member name against dest. It reports skip=true for a bare
// directory marker (empty relative path) and an error for any path that escapes dest
// (absolute, drive prefix, or ".."). isDir is true when the member name ends with a
// path separator.
func memberTarget(archivePath, dest, name string) (target string, isDir, skip bool, err error) {
	cleaned := strings.TrimSuffix(name, "/")
	trailingSlash := cleaned != name
	if cleaned == "" {
		return "", false, true, nil
	}
	components := 0
	result := dest
	for _, part := range strings.Split(filepath.ToSlash(cleaned), "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", false, false, escapeError(archivePath, name)
		default:
			if strings.Contains(part, ":") || filepath.IsAbs(part) {
				return "", false, false, escapeError(archivePath, name)
			}
			result = filepath.Join(result, part)
			components++
		}
	}
	if components == 0 {
		return "", false, true, nil
	}
	return result, trailingSlash, false, nil
}

func extractDir(archivePath, root, target, member string, mode os.FileMode) error {
	if err := createAllDir(target); err != nil {
		return err
	}
	if err := ensureWithinRoot(archivePath, root, target, member); err != nil {
		return err
	}
	return applyArchiveMode(archivePath, target, mode)
}

func extractFile(archivePath, root, target, member string, reader io.Reader, limits ExtractLimits, written *int64, mode os.FileMode) error {
	parent := filepath.Dir(target)
	if err := createAllDir(parent); err != nil {
		return err
	}
	if err := ensureWithinRoot(archivePath, root, parent, member); err != nil {
		return err
	}
	if err := ensureLeafNotSymlink(archivePath, target, member); err != nil {
		return err
	}
	if err := writeMemberBounded(archivePath, target, reader, limits, written); err != nil {
		return err
	}
	return applyArchiveMode(archivePath, target, mode)
}

// writeMemberBounded writes a member's contents to target, streaming through a reader
// capped at the remaining byte budget so a decompression bomb cannot exceed
// limits.MaxTotalBytes. It reads one byte past the budget so an overrun is detected
// without trusting the member's declared size.
func writeMemberBounded(archivePath, target string, reader io.Reader, limits ExtractLimits, written *int64) error {
	remaining := limits.MaxTotalBytes - *written
	if remaining < 0 {
		remaining = 0
	}
	out, err := os.Create(target)
	if err != nil {
		return extractIOError(archivePath, "create member", err)
	}
	limited := io.LimitReader(reader, remaining+1)
	copied, err := io.Copy(out, limited)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return extractIOError(archivePath, "write member", err)
	}
	if copied > remaining {
		_ = os.Remove(target)
		return oversizeError(archivePath, limits.MaxTotalBytes)
	}
	*written += copied
	return nil
}

func openArchive(archivePath string) (*os.File, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.InvalidInput("archive",
				fmt.Sprintf("cannot open archive '%s': %v", archivePath, err)).WithCause(err)
		}
		code, status := osErrorCode(err)
		return nil, apperrors.New(code,
			fmt.Sprintf("cannot open archive '%s': %v", archivePath, err), status).WithCause(err)
	}
	return file, nil
}

// extractionRoot ensures dest exists and resolves it to a canonical containment root,
// so a symlink already present under dest cannot redirect a write outside it.
func extractionRoot(archivePath, dest string) (string, error) {
	if err := createAllDir(dest); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return "", extractIOError(archivePath, "resolve destination", err)
	}
	return resolved, nil
}

// ensureWithinRoot rejects a member whose materialized dir resolves outside root,
// defending against pre-existing symlinks in the destination tree.
func ensureWithinRoot(archivePath, root, dir, member string) error {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return extractIOError(archivePath, "resolve member path", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return escapeError(archivePath, member)
	}
	return nil
}

// ensureLeafNotSymlink rejects a file member whose destination leaf already exists
// as a symlink. The caller has confined the leaf's parent directory within root, so
// the only remaining redirection is a symlink at the leaf itself: os.Create would
// follow it and write through to the link target outside dest. A missing leaf is the
// normal case; a regular file is left for the bounded writer to truncate and replace.
func ensureLeafNotSymlink(archivePath, target, member string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return extractIOError(archivePath, "inspect member target", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return escapeError(archivePath, member)
	}
	return nil
}

func createAllDir(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("cannot create directory '%s': %v", dir, err), 500).WithCause(err)
	}
	return nil
}

func escapeError(archivePath, member string) error {
	return apperrors.InvalidInput("archive",
		fmt.Sprintf("archive '%s' contains an unsafe member path '%s'", archivePath, member))
}

func unsafeLinkError(archivePath, member string) error {
	return apperrors.InvalidInput("archive",
		fmt.Sprintf("archive '%s' contains a link member '%s', which is not permitted", archivePath, member))
}

func oversizeError(archivePath string, maxTotalBytes int64) error {
	return apperrors.InvalidInput("archive",
		fmt.Sprintf("archive '%s' exceeds the extraction limit of %d uncompressed bytes", archivePath, maxTotalBytes))
}

func tooManyEntriesError(archivePath string, maxEntries int) error {
	return apperrors.InvalidInput("archive",
		fmt.Sprintf("archive '%s' exceeds the extraction limit of %d members", archivePath, maxEntries))
}

func extractIOError(archivePath, action string, err error) error {
	return apperrors.New(apperrors.ErrCodeInternal,
		fmt.Sprintf("cannot %s for '%s': %v", action, archivePath, err), 500).WithCause(err)
}
