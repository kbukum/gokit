package gcs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/storage"
)

func TestHeadUsesInjectedClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := newFakeClient()
	s := NewStorageWithClient("bucket", "https://cdn.example", client)
	if err := s.Upload(ctx, "dir/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	info, err := s.Head(ctx, "dir/a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != 5 || info.Checksum == "" {
		t.Fatalf("info = %#v", info)
	}
}

func TestHeadMissingErrors(t *testing.T) {
	t.Parallel()
	s := NewStorageWithClient("bucket", "", newFakeClient())
	_, err := s.Head(context.Background(), "absent")
	if err == nil {
		t.Fatal("Head for a missing object did not error")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Head error is not storage.ErrNotFound: %v", err)
	}
}

func TestCopyRenameRejectSamePath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := newFakeClient()
	s := NewStorageWithClient("bucket", "", client)
	if err := s.Upload(ctx, "a.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := s.Copy(ctx, "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Copy same path = %v, want storage.ErrSameObject", err)
	}
	if err := s.Rename(ctx, "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Rename same path = %v, want storage.ErrSameObject", err)
	}
	if client.objects["a.txt"] != "payload" {
		t.Errorf("object mutated by rejected same-path op: %q", client.objects["a.txt"])
	}
}

func TestCopyDuplicatesObject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := newFakeClient()
	s := NewStorageWithClient("bucket", "", client)
	if err := s.Upload(ctx, "src.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := s.Copy(ctx, "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if client.objects["dst.txt"] != "payload" || client.objects["src.txt"] != "payload" {
		t.Fatalf("objects = %#v", client.objects)
	}
}

func TestRenameMovesObject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := newFakeClient()
	s := NewStorageWithClient("bucket", "", client)
	if err := s.Upload(ctx, "src.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := s.Rename(ctx, "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if client.objects["dst.txt"] != "payload" {
		t.Fatalf("dst = %q", client.objects["dst.txt"])
	}
	if _, ok := client.objects["src.txt"]; ok {
		t.Fatal("source still present after rename")
	}
}
