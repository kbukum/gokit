package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/storage"
)

func startComponent(t *testing.T) (*Component, context.Context) {
	t.Helper()
	comp := NewComponent()
	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = comp.Stop(ctx) })
	return comp, ctx
}

func TestComponent_HeadReturnsMetadataAndChecksum(t *testing.T) {
	comp, ctx := startComponent(t)
	const payload = "hello world"
	if err := comp.Upload(ctx, "a.txt", strings.NewReader(payload)); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	info, err := comp.Head(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Errorf("Size = %d, want %d", info.Size, len(payload))
	}
	sum := sha256.Sum256([]byte(payload))
	if want := hex.EncodeToString(sum[:]); info.Checksum != want {
		t.Errorf("Checksum = %q, want %q", info.Checksum, want)
	}
}

func TestComponent_HeadMissingErrors(t *testing.T) {
	comp, ctx := startComponent(t)
	if _, err := comp.Head(ctx, "absent"); err == nil {
		t.Fatal("Head for a missing object did not error")
	}
}

func TestComponent_ChecksumIsStable(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Upload(ctx, "a.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	first, err := comp.Head(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if err := comp.Upload(ctx, "a.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("re-Upload: %v", err)
	}
	second, err := comp.Head(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if first.Checksum == "" || first.Checksum != second.Checksum {
		t.Fatalf("checksum not stable: %q vs %q", first.Checksum, second.Checksum)
	}
}

func TestComponent_CopyDuplicatesAndKeepsSource(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Upload(ctx, "src.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := comp.Copy(ctx, "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	assertContent(t, comp, ctx, "dst.txt", "payload")
	assertContent(t, comp, ctx, "src.txt", "payload")
}

func TestComponent_CopyMissingSourceErrors(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Copy(ctx, "absent", "dst"); err == nil {
		t.Fatal("Copy of a missing source did not error")
	}
}

func TestComponent_RenameMovesAndRemovesSource(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Upload(ctx, "src.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := comp.Rename(ctx, "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	assertContent(t, comp, ctx, "dst.txt", "payload")
	if ok, err := comp.Exists(ctx, "src.txt"); err != nil || ok {
		t.Fatalf("source still present: ok=%v err=%v", ok, err)
	}
}

func TestComponent_RenameMissingSourceErrors(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Rename(ctx, "absent", "dst"); err == nil {
		t.Fatal("Rename of a missing source did not error")
	}
}

func TestComponent_MissingReturnsErrNotFound(t *testing.T) {
	comp, ctx := startComponent(t)
	if _, err := comp.Head(ctx, "absent"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Head error is not storage.ErrNotFound: %v", err)
	}
}

func TestComponent_CopyRenameRejectSamePath(t *testing.T) {
	comp, ctx := startComponent(t)
	if err := comp.Upload(ctx, "a.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := comp.Copy(ctx, "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Copy same path = %v, want storage.ErrSameObject", err)
	}
	if err := comp.Rename(ctx, "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Rename same path = %v, want storage.ErrSameObject", err)
	}
	assertContent(t, comp, ctx, "a.txt", "payload")
}

func assertContent(t *testing.T, comp *Component, ctx context.Context, path, want string) {
	t.Helper()
	rc, err := comp.Download(ctx, path)
	if err != nil {
		t.Fatalf("Download %q: %v", path, err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll %q: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content of %q = %q, want %q", path, got, want)
	}
}
