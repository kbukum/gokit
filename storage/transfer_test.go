package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
)

var errMissing = errors.New("object not found")

// mapStore is a minimal in-memory Storage used to exercise cross-store helpers.
// It deliberately does not implement StatProvider.
type mapStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMapStore() *mapStore {
	return &mapStore{files: map[string][]byte{}}
}

// statStore is a mapStore that also implements StatProvider.
type statStore struct{ *mapStore }

func newStatStore() statStore {
	return statStore{newMapStore()}
}

func (m *mapStore) Upload(_ context.Context, path string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = data
	return nil
}

func (m *mapStore) Download(_ context.Context, path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[path]
	if !ok {
		return nil, errMissing
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), data...))), nil
}

func (m *mapStore) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	return nil
}

func (m *mapStore) Exists(_ context.Context, path string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[path]
	return ok, nil
}

func (m *mapStore) URL(_ context.Context, path string) (string, error) { return "mem://" + path, nil }

func (m *mapStore) List(_ context.Context, prefix string) ([]FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []FileInfo
	for p, data := range m.files {
		if strings.HasPrefix(p, prefix) {
			out = append(out, FileInfo{Path: p, Size: int64(len(data))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (m statStore) Stat(_ context.Context, path string) (FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[path]
	if !ok {
		return FileInfo{}, errMissing
	}
	return FileInfo{Path: path, Size: int64(len(data)), ContentType: "application/octet-stream"}, nil
}

func TestTransferCopiesBytesBetweenStores(t *testing.T) {
	t.Parallel()

	src := newMapStore()
	dst := newMapStore()
	ctx := context.Background()
	if err := src.Upload(ctx, "a.txt", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatalf("seed upload: %v", err)
	}

	if err := Transfer(ctx, src, "a.txt", dst, "b.txt"); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	rc, err := dst.Download(ctx, "b.txt")
	if err != nil {
		t.Fatalf("Download dst: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "hello" {
		t.Fatalf("transferred content = %q, want %q", got, "hello")
	}
}

func TestTransferMissingSourceErrors(t *testing.T) {
	t.Parallel()

	err := Transfer(context.Background(), newMapStore(), "absent", newMapStore(), "x")
	if err == nil {
		t.Fatal("Transfer from a missing source did not error")
	}
}

func TestTransferValidatesArguments(t *testing.T) {
	t.Parallel()

	s := newMapStore()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		src, dst         Storage
		srcPath, dstPath string
	}{
		"nil src":       {nil, s, "a", "b"},
		"nil dst":       {s, nil, "a", "b"},
		"empty srcPath": {s, s, "", "b"},
		"empty dstPath": {s, s, "a", ""},
	} {
		if err := Transfer(ctx, tc.src, tc.srcPath, tc.dst, tc.dstPath); err == nil {
			t.Errorf("%s: Transfer did not error", name)
		}
	}
}

func TestStatUsesStatProviderWhenAvailable(t *testing.T) {
	t.Parallel()

	s := newStatStore()
	ctx := context.Background()
	if err := s.Upload(ctx, "a.txt", bytes.NewReader([]byte("hi"))); err != nil {
		t.Fatalf("upload: %v", err)
	}
	info, err := Stat(ctx, s, "a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.ContentType != "application/octet-stream" {
		t.Fatalf("Stat did not use StatProvider (ContentType = %q)", info.ContentType)
	}
}

func TestStatFallsBackToList(t *testing.T) {
	t.Parallel()

	s := newMapStore() // no StatProvider
	ctx := context.Background()
	if err := s.Upload(ctx, "dir/a.txt", bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("upload: %v", err)
	}
	info, err := Stat(ctx, s, "dir/a.txt")
	if err != nil {
		t.Fatalf("Stat fallback: %v", err)
	}
	if info.Size != 4 {
		t.Fatalf("Stat fallback Size = %d, want 4", info.Size)
	}
}

func TestStatNotFound(t *testing.T) {
	t.Parallel()

	if _, err := Stat(context.Background(), newMapStore(), "absent"); err == nil {
		t.Fatal("Stat for a missing object did not error")
	}
}
