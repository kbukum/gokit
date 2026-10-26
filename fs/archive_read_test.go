package fs_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
)

type zipFile struct {
	name   string
	body   string
	method uint16
	mode   os.FileMode
}

func buildZip(t *testing.T, files ...zipFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: f.method}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadZip(t *testing.T) {
	t.Parallel()
	data := buildZip(t,
		zipFile{name: "cases.jsonl", body: `{"id":"a"}`, method: zip.Deflate},
		zipFile{name: "media/", method: zip.Store},
		zipFile{name: "media/cat.png", body: "png bytes", method: zip.Store},
	)
	got, err := fs.ReadZip(data, fs.ZipReadLimits{})
	if err != nil {
		t.Fatalf("ReadZip: %v", err)
	}
	if len(got) != 2 || got[0].Name != "cases.jsonl" || string(got[0].Data) != `{"id":"a"}` || got[1].Name != "media/cat.png" {
		t.Fatalf("ReadZip = %+v", got)
	}
}

func TestReadZipRefuses(t *testing.T) {
	t.Parallel()
	bomb := strings.Repeat("0", 1<<20)
	for name, tc := range map[string]struct {
		data   []byte
		limits fs.ZipReadLimits
		want   error
	}{
		"not a zip":       {[]byte("hello"), fs.ZipReadLimits{}, fs.ErrArchiveMalformed},
		"too many":        {buildZip(t, zipFile{name: "a"}, zipFile{name: "b"}, zipFile{name: "c"}), fs.ZipReadLimits{MaxEntries: 2}, fs.ErrArchiveTooManyEntries},
		"entry too large": {buildZip(t, zipFile{name: "a", body: "12345"}), fs.ZipReadLimits{MaxEntryBytes: 4}, fs.ErrArchiveTooLarge},
		"total too large": {buildZip(t, zipFile{name: "a", body: "123"}, zipFile{name: "b", body: "123"}), fs.ZipReadLimits{MaxTotalBytes: 5}, fs.ErrArchiveTooLarge},
		"zip bomb":        {buildZip(t, zipFile{name: "bomb", body: bomb, method: zip.Deflate}), fs.ZipReadLimits{}, fs.ErrArchiveRatio},
		"parent path":     {buildZip(t, zipFile{name: "../evil"}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
		"absolute path":   {buildZip(t, zipFile{name: "/etc/passwd"}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
		"backslash":       {buildZip(t, zipFile{name: `a\..\..\evil`}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
		"control char":    {buildZip(t, zipFile{name: "a\x00b"}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
		"duplicate":       {buildZip(t, zipFile{name: "a.png"}, zipFile{name: "A.PNG"}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
		"symlink":         {buildZip(t, zipFile{name: "link", body: "/etc/passwd", mode: os.ModeSymlink | 0o777}), fs.ZipReadLimits{}, fs.ErrArchiveUnsafeMember},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := fs.ReadZip(tc.data, tc.limits); !errors.Is(err, tc.want) {
				t.Fatalf("ReadZip = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestReadZipRejectsBadLimits(t *testing.T) {
	t.Parallel()
	data := buildZip(t, zipFile{name: "a"})
	for _, l := range []fs.ZipReadLimits{{MaxEntries: -1}, {MaxTotalBytes: -1}, {MaxEntryBytes: -1}, {MaxRatio: -1}} {
		if _, err := fs.ReadZip(data, l); err == nil {
			t.Fatalf("limits %+v: want an error", l)
		}
	}
}

func FuzzReadZip(f *testing.F) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("a.txt")
	_, _ = w.Write([]byte("hello"))
	_ = zw.Close()
	f.Add(buf.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		files, err := fs.ReadZip(data, fs.ZipReadLimits{MaxEntries: 8, MaxTotalBytes: 1 << 16})
		if err != nil {
			return
		}
		var total int
		for _, f := range files {
			total += len(f.Data)
		}
		if len(files) > 8 || total > 1<<16 {
			t.Fatalf("read %d files, %d bytes, past the limits", len(files), total)
		}
	})
}
