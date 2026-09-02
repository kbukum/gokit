package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// bareStore exposes only the required Storage methods, so the capability
// fallbacks in Head, Copy, and Rename are exercised.
type bareStore struct{ inner *mapStore }

func newBareStore() *bareStore { return &bareStore{inner: newMapStore()} }

func (b *bareStore) Upload(ctx context.Context, path string, reader io.Reader) error {
	return b.inner.Upload(ctx, path, reader)
}

func (b *bareStore) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	return b.inner.Download(ctx, path)
}

func (b *bareStore) Delete(ctx context.Context, path string) error {
	return b.inner.Delete(ctx, path)
}

func (b *bareStore) Exists(ctx context.Context, path string) (bool, error) {
	return b.inner.Exists(ctx, path)
}

func (b *bareStore) URL(ctx context.Context, path string) (string, error) {
	return b.inner.URL(ctx, path)
}

func (b *bareStore) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	return b.inner.List(ctx, prefix)
}

func seed(t *testing.T, s Storage, path, content string) {
	t.Helper()
	if err := s.Upload(context.Background(), path, bytes.NewReader([]byte(content))); err != nil {
		t.Fatalf("seed upload %q: %v", path, err)
	}
}

func readAll(t *testing.T, s Storage, path string) string {
	t.Helper()
	rc, err := s.Download(context.Background(), path)
	if err != nil {
		t.Fatalf("download %q: %v", path, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(data)
}

func TestHeadUsesProviderWhenAvailable(t *testing.T) {
	t.Parallel()

	s := newMapStore()
	seed(t, s, "a.txt", "hi")
	info, err := Head(context.Background(), s, "a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != 2 || info.ContentType != "application/octet-stream" {
		t.Fatalf("Head = %#v, want the provider result", info)
	}
}

func TestHeadFallsBackToList(t *testing.T) {
	t.Parallel()

	s := newBareStore()
	seed(t, s, "a.txt", "hi")
	info, err := Head(context.Background(), s, "a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Path != "a.txt" || info.Size != 2 {
		t.Fatalf("Head = %#v, want the listing fallback result", info)
	}
}

func TestHeadFallbackRejectsPrefixMatch(t *testing.T) {
	t.Parallel()

	s := newBareStore()
	seed(t, s, "a.txt.bak", "hi")
	if _, err := Head(context.Background(), s, "a.txt"); err == nil {
		t.Fatal("Head matched a prefix rather than the exact path")
	}
}

func TestHeadNotFound(t *testing.T) {
	t.Parallel()

	for name, s := range map[string]Storage{"provider": newMapStore(), "fallback": newBareStore()} {
		_, err := Head(context.Background(), s, "absent")
		if err == nil {
			t.Errorf("%s: Head for a missing object did not error", name)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: Head error is not ErrNotFound: %v", name, err)
		}
	}
}

func TestHeadValidatesArguments(t *testing.T) {
	t.Parallel()

	if _, err := Head(context.Background(), nil, "a"); err == nil {
		t.Error("Head with a nil store did not error")
	}
	if _, err := Head(context.Background(), newMapStore(), ""); err == nil {
		t.Error("Head with an empty path did not error")
	}
}

func TestCopyKeepsSourceInPlace(t *testing.T) {
	t.Parallel()

	for name, s := range map[string]Storage{"provider": newMapStore(), "fallback": newBareStore()} {
		seed(t, s, "src.txt", "payload")
		if err := Copy(context.Background(), s, "src.txt", "dst.txt"); err != nil {
			t.Fatalf("%s: Copy: %v", name, err)
		}
		if got := readAll(t, s, "dst.txt"); got != "payload" {
			t.Errorf("%s: destination = %q, want %q", name, got, "payload")
		}
		if got := readAll(t, s, "src.txt"); got != "payload" {
			t.Errorf("%s: source = %q, want it left in place", name, got)
		}
	}
}

func TestCopyMissingSourceErrors(t *testing.T) {
	t.Parallel()

	for name, s := range map[string]Storage{"provider": newMapStore(), "fallback": newBareStore()} {
		if err := Copy(context.Background(), s, "absent", "dst.txt"); err == nil {
			t.Errorf("%s: Copy from a missing source did not error", name)
		}
	}
}

func TestRenameMovesObject(t *testing.T) {
	t.Parallel()

	for name, s := range map[string]Storage{"provider": newMapStore(), "fallback": newBareStore()} {
		seed(t, s, "src.txt", "payload")
		if err := Rename(context.Background(), s, "src.txt", "dst.txt"); err != nil {
			t.Fatalf("%s: Rename: %v", name, err)
		}
		if got := readAll(t, s, "dst.txt"); got != "payload" {
			t.Errorf("%s: destination = %q, want %q", name, got, "payload")
		}
		exists, err := s.Exists(context.Background(), "src.txt")
		if err != nil {
			t.Fatalf("%s: Exists: %v", name, err)
		}
		if exists {
			t.Errorf("%s: source still present after Rename", name)
		}
	}
}

func TestRenameMissingSourceErrors(t *testing.T) {
	t.Parallel()

	for name, s := range map[string]Storage{"provider": newMapStore(), "fallback": newBareStore()} {
		if err := Rename(context.Background(), s, "absent", "dst.txt"); err == nil {
			t.Errorf("%s: Rename from a missing source did not error", name)
		}
	}
}

func TestObjectOpsValidateArguments(t *testing.T) {
	t.Parallel()

	s := newMapStore()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		store            Storage
		srcPath, dstPath string
	}{
		"nil store":     {nil, "a", "b"},
		"empty srcPath": {s, "", "b"},
		"empty dstPath": {s, "a", ""},
	} {
		if err := Copy(ctx, tc.store, tc.srcPath, tc.dstPath); err == nil {
			t.Errorf("%s: Copy did not error", name)
		}
		if err := Rename(ctx, tc.store, tc.srcPath, tc.dstPath); err == nil {
			t.Errorf("%s: Rename did not error", name)
		}
	}
}

func TestObjectOpsRejectSamePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newMapStore()
	seed(t, s, "a.txt", "payload")

	if err := Copy(ctx, s, "a.txt", "a.txt"); !errors.Is(err, ErrSameObject) {
		t.Errorf("Copy same path = %v, want ErrSameObject", err)
	}
	if err := Rename(ctx, s, "a.txt", "a.txt"); !errors.Is(err, ErrSameObject) {
		t.Errorf("Rename same path = %v, want ErrSameObject", err)
	}
	if got := readAll(t, s, "a.txt"); got != "payload" {
		t.Errorf("object mutated by rejected same-path op: %q", got)
	}
}
