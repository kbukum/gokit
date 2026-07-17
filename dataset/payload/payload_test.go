package payload

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

func TestPayloadFromBytes(t *testing.T) {
	t.Parallel()
	p, err := FromBytes([]byte("hello"), DefaultLimits())
	if err != nil {
		t.Fatalf("FromBytes error: %v", err)
	}
	if p.IsFile() {
		t.Fatal("in-memory payload should not report IsFile")
	}
	got, err := p.ReadBounded(DefaultLimits())
	if err != nil {
		t.Fatalf("ReadBounded error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadBounded = %q; want hello", got)
	}
}

func TestPayloadFromBytesRejectsOversize(t *testing.T) {
	t.Parallel()
	_, err := FromBytes([]byte("toolong"), Limits{MaxInMemoryBytes: 3})
	if err == nil {
		t.Fatal("expected error for oversize payload")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestPayloadFromFileReadBounded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.bin")
	if err := os.WriteFile(path, []byte("filedata"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := FromFile(path)
	if !p.IsFile() {
		t.Fatal("file payload should report IsFile")
	}
	got, err := p.ReadBounded(DefaultLimits())
	if err != nil {
		t.Fatalf("ReadBounded error: %v", err)
	}
	if string(got) != "filedata" {
		t.Fatalf("ReadBounded = %q", got)
	}
	if _, err := p.ReadBounded(Limits{MaxInMemoryBytes: 2}); !errors.Is(err, fs.ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestPayloadWriteTo(t *testing.T) {
	t.Parallel()
	p, _ := FromBytes([]byte("abc"), DefaultLimits())
	var buf bytes.Buffer
	n, err := p.WriteTo(&buf)
	if err != nil || n != 3 || buf.String() != "abc" {
		t.Fatalf("WriteTo = %d, %v, %q", n, err, buf.String())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "w.bin")
	if err := os.WriteFile(path, []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fbuf bytes.Buffer
	fn, err := FromFile(path).WriteTo(&fbuf)
	if err != nil || fn != 3 || fbuf.String() != "xyz" {
		t.Fatalf("file WriteTo = %d, %v, %q", fn, err, fbuf.String())
	}
}

func TestPayloadInMemoryOverCapReadRejected(t *testing.T) {
	t.Parallel()
	p := Payload{data: []byte("large")}
	if _, err := p.ReadBounded(Limits{MaxInMemoryBytes: 2}); err == nil {
		t.Fatal("expected error reading in-memory payload over cap")
	}
}
