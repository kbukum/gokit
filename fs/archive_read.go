package fs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	apperrors "github.com/kbukum/gokit/errors"
)

// Defaults applied by [ReadZip] when a [ZipReadLimits] field is zero.
const (
	DefaultZipMaxEntries    = 1024
	DefaultZipMaxTotalBytes = 64 << 20
	DefaultZipMaxEntryBytes = 16 << 20
	DefaultZipMaxRatio      = 100
	maxZipMemberName        = 512
)

// Sentinel errors [ReadZip] returns for a refused archive, matched with errors.Is.
var (
	// ErrArchiveMalformed means the bytes are not a readable zip archive.
	ErrArchiveMalformed = errors.New("archive is malformed")
	// ErrArchiveTooManyEntries means the archive holds more members than allowed.
	ErrArchiveTooManyEntries = errors.New("archive has too many members")
	// ErrArchiveTooLarge means a member, or all members together, decompress past the byte limit.
	ErrArchiveTooLarge = errors.New("archive decompresses past the byte limit")
	// ErrArchiveRatio means a member decompresses to more than MaxRatio times its compressed size.
	ErrArchiveRatio = errors.New("archive member exceeds the compression ratio limit")
	// ErrArchiveUnsafeMember means a member name is not a plain relative path, repeats another name, or the member is
	// a link, a device or encrypted.
	ErrArchiveUnsafeMember = errors.New("archive has an unsafe member")
)

// ZipReadLimits bounds [ReadZip]. A zero field takes its default; a negative one is invalid.
type ZipReadLimits struct {
	// MaxEntries bounds the members, directories included. Zero means [DefaultZipMaxEntries].
	MaxEntries int
	// MaxTotalBytes bounds the bytes all members decompress to. Zero means [DefaultZipMaxTotalBytes].
	MaxTotalBytes int64
	// MaxEntryBytes bounds the bytes one member decompresses to. Zero means [DefaultZipMaxEntryBytes].
	MaxEntryBytes int64
	// MaxRatio bounds a member's decompressed size as a multiple of its compressed size. Zero means
	// [DefaultZipMaxRatio].
	MaxRatio int64
}

func (l ZipReadLimits) normalized() (ZipReadLimits, error) {
	if l.MaxEntries < 0 || l.MaxTotalBytes < 0 || l.MaxEntryBytes < 0 || l.MaxRatio < 0 {
		return l, apperrors.InvalidInput("limits", "zip read limits must not be negative")
	}
	if l.MaxEntries == 0 {
		l.MaxEntries = DefaultZipMaxEntries
	}
	if l.MaxTotalBytes == 0 {
		l.MaxTotalBytes = DefaultZipMaxTotalBytes
	}
	if l.MaxEntryBytes == 0 {
		l.MaxEntryBytes = DefaultZipMaxEntryBytes
	}
	if l.MaxRatio == 0 {
		l.MaxRatio = DefaultZipMaxRatio
	}
	return l, nil
}

// ZipMember is one regular file [ReadZip] read.
type ZipMember struct {
	Name string // A clean, slash-separated relative path
	Data []byte
}

// ReadZip reads an untrusted zip archive held in memory and returns its regular files in archive order, skipping
// directories. Nothing touches the filesystem. Every member streams through a reader capped by the remaining budget,
// so a member's declared size is never trusted: the archive is refused when it has too many members, a member or
// the whole archive decompresses past its byte limit, or a member decompresses past MaxRatio times its compressed
// size. A member whose name is absolute, climbs with "..", uses a backslash or a control character, or repeats
// another name ignoring case is refused, as is a link, a device or an encrypted member. A refusal matches one of
// the ErrArchive sentinels.
func ReadZip(data []byte, limits ZipReadLimits) ([]ZipMember, error) {
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrArchiveMalformed, err)
	}
	if len(zr.File) > l.MaxEntries {
		return nil, fmt.Errorf("%w: more than %d", ErrArchiveTooManyEntries, l.MaxEntries)
	}
	seen := make(map[string]bool, len(zr.File))
	var out []ZipMember
	var total int64
	for _, f := range zr.File {
		name, dir, err := zipMemberName(f)
		if err != nil {
			return nil, err
		}
		folded := strings.ToLower(name)
		if seen[folded] {
			return nil, fmt.Errorf("%w: %q repeats another name", ErrArchiveUnsafeMember, name)
		}
		seen[folded] = true
		if dir {
			continue
		}
		body, err := readZipMember(f, name, min(l.MaxEntryBytes, l.MaxTotalBytes-total), l)
		if err != nil {
			return nil, err
		}
		total += int64(len(body))
		out = append(out, ZipMember{Name: name, Data: body})
	}
	return out, nil
}

func zipMemberName(f *zip.File) (name string, dir bool, err error) {
	unsafe := func(why string) (string, bool, error) {
		return "", false, fmt.Errorf("%w: %q %s", ErrArchiveUnsafeMember, f.Name, why)
	}
	mode := f.Mode()
	switch {
	case f.Flags&0x1 != 0:
		return unsafe("is encrypted")
	case mode.Type() != 0 && !mode.IsDir():
		return unsafe("is not a regular file")
	case f.Name == "" || len(f.Name) > maxZipMemberName:
		return unsafe("has an empty or overlong name")
	case strings.ContainsRune(f.Name, '\\') || strings.IndexFunc(f.Name, unicode.IsControl) >= 0:
		return unsafe("has a backslash or a control character")
	}
	dir = strings.HasSuffix(f.Name, "/") || mode.IsDir()
	trimmed := strings.TrimSuffix(f.Name, "/")
	if trimmed == "" || path.IsAbs(trimmed) || path.Clean(trimmed) != trimmed || strings.Contains(trimmed, ":") {
		return unsafe("is not a clean relative path")
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." || part == "." {
			return unsafe("is not a clean relative path")
		}
	}
	return trimmed, dir, nil
}

func readZipMember(f *zip.File, name string, budget int64, l ZipReadLimits) ([]byte, error) {
	limit, refusal := budget, ErrArchiveTooLarge
	if c := f.CompressedSize64; c <= uint64(budget/l.MaxRatio) && int64(c)*l.MaxRatio < limit {
		limit, refusal = int64(c)*l.MaxRatio, ErrArchiveRatio
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %w", ErrArchiveMalformed, name, err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %q: %w", ErrArchiveMalformed, name, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %q", refusal, name)
	}
	return body, nil
}
