package gcs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/storage"
)

type fakeObjectClient struct {
	objects map[string]string
	fail    error
}

func newFakeClient() *fakeObjectClient { return &fakeObjectClient{objects: map[string]string{}} }

func (f *fakeObjectClient) Put(_ context.Context, path string, r io.Reader) error {
	if f.fail != nil {
		return f.fail
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[path] = string(b)
	return nil
}

func (f *fakeObjectClient) Get(_ context.Context, path string) (io.ReadCloser, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	v, ok := f.objects[path]
	if !ok {
		return nil, errors.New("missing")
	}
	return io.NopCloser(strings.NewReader(v)), nil
}

func (f *fakeObjectClient) Delete(_ context.Context, path string) error {
	if f.fail != nil {
		return f.fail
	}
	delete(f.objects, path)
	return nil
}

func (f *fakeObjectClient) Exists(_ context.Context, path string) (bool, error) {
	if f.fail != nil {
		return false, f.fail
	}
	_, ok := f.objects[path]
	return ok, nil
}

func (f *fakeObjectClient) List(_ context.Context, prefix string) ([]storage.FileInfo, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	files := make([]storage.FileInfo, 0)
	for path, body := range f.objects {
		if strings.HasPrefix(path, prefix) {
			files = append(files, storage.FileInfo{Path: path, Size: int64(len(body))})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func (f *fakeObjectClient) SignedURL(_ context.Context, path string, _ time.Duration) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	return "https://signed.example/" + path, nil
}

func TestStorageOperationsUseInjectedClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := newFakeClient()
	s := NewStorageWithClient("bucket", "https://cdn.example", client)
	if err := s.Upload(ctx, "dir/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	exists, err := s.Exists(ctx, "dir/a.txt")
	if err != nil || !exists {
		t.Fatalf("Exists = %v, %v", exists, err)
	}
	body, err := s.Download(ctx, "dir/a.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := io.ReadAll(body)
	if closeErr := body.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if err != nil || string(got) != "hello" {
		t.Fatalf("body = %q, %v", got, err)
	}
	files, err := s.List(ctx, "dir/")
	if err != nil || len(files) != 1 || files[0].Path != "dir/a.txt" {
		t.Fatalf("List = %#v, %v", files, err)
	}
	url, err := s.URL(ctx, "dir/a.txt")
	if err != nil || url != "https://cdn.example/dir%2Fa.txt" {
		t.Fatalf("URL = %q, %v", url, err)
	}
	signed, err := s.SignedURL(ctx, "dir/a.txt", time.Minute)
	if err != nil || signed == "" {
		t.Fatalf("SignedURL = %q, %v", signed, err)
	}
	if err := s.Delete(ctx, "dir/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRegisterCapturesTypedConfig(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{Bucket: "objects"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get(ProviderName); !ok {
		t.Fatal("gcs provider missing")
	}
}

func TestStorageWrapsClientErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	s := NewStorageWithClient("bucket", "", &fakeObjectClient{fail: boom})
	if err := s.Upload(context.Background(), "a", strings.NewReader("x")); !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom, got %v", err)
	}
}

func TestStorageDefaultURLAndGetBucket(t *testing.T) {
	t.Parallel()
	cfg := Config{Bucket: "objects"}
	if got := cfg.GetBucket(); got != "objects" {
		t.Fatalf("GetBucket = %q", got)
	}
	s := NewStorageWithClient("objects", "", newFakeClient())
	got, err := s.URL(context.Background(), "dir/a.txt")
	if err != nil || got != "https://storage.googleapis.com/objects/dir%2Fa.txt" {
		t.Fatalf("URL = %q, %v", got, err)
	}
}

func TestNewStorageUsesEmulatorClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL)
	cfg := Config{Bucket: "objects"}
	cfg.ApplyDefaults()
	store, err := NewStorage(context.Background(), &cfg)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if store == nil {
		t.Fatal("expected store")
	}
}

func TestRegisterRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	if err := Register(storage.NewFactoryRegistry(), Config{}); err == nil {
		t.Fatal("expected invalid register config")
	}
}

func TestStorageWrapsAllClientErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	s := NewStorageWithClient("bucket", "", &fakeObjectClient{fail: boom})
	ctx := context.Background()
	if _, err := s.Download(ctx, "a"); !errors.Is(err, boom) {
		t.Fatalf("Download err = %v", err)
	}
	if err := s.Delete(ctx, "a"); !errors.Is(err, boom) {
		t.Fatalf("Delete err = %v", err)
	}
	if _, err := s.Exists(ctx, "a"); !errors.Is(err, boom) {
		t.Fatalf("Exists err = %v", err)
	}
	if _, err := s.List(ctx, "a"); !errors.Is(err, boom) {
		t.Fatalf("List err = %v", err)
	}
	if _, err := s.SignedURL(ctx, "a", time.Minute); !errors.Is(err, boom) {
		t.Fatalf("SignedURL err = %v", err)
	}
}

func TestRegisteredFactoryConstructsStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL)

	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{Bucket: "objects"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	factory, ok := reg.Get(ProviderName)
	if !ok {
		t.Fatal("gcs provider missing")
	}
	store, err := factory(storage.Config{}, nil)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if store == nil {
		t.Fatal("expected store from factory")
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()
	if err := Register(nil, Config{Bucket: "bucket"}); err == nil {
		t.Fatal("expected nil registry error")
	}
}
