package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/storage"
)

func TestHeadReturnsMetadata(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/storage/v1/object/info/bucket/") {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"a.txt","metadata":{"size":5,"mimetype":"text/plain","eTag":"\"abc123\""},"updated_at":"2023-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	info, err := s.Head(context.Background(), "dir/a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != 5 || info.ContentType != "text/plain" {
		t.Fatalf("info = %#v", info)
	}
	if info.Checksum != "" {
		t.Errorf("Checksum = %q, want empty: the Supabase eTag is not a verified content hash", info.Checksum)
	}
	if info.LastModified.IsZero() {
		t.Error("expected parsed LastModified")
	}
}

func TestHeadNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	_, err := s.Head(context.Background(), "absent")
	if err == nil {
		t.Fatal("Head for a missing object did not error")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Head error is not storage.ErrNotFound: %v", err)
	}
}

func TestHeadRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A single JSON string value larger than the metadata decode limit must
		// be rejected rather than read unbounded into memory.
		_, _ = io.WriteString(w, `{"name":"`)
		_, _ = io.WriteString(w, strings.Repeat("a", maxMetadataBodyBytes+(1<<20)))
		_, _ = io.WriteString(w, `","metadata":{"size":1}}`)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.Head(context.Background(), "dir/a.txt"); err == nil {
		t.Fatal("Head must reject a response body exceeding the metadata decode limit")
	}
}

func TestCopyRenameRejectSamePath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("same-path op must not issue a request, got %s", r.URL.Path)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Copy(context.Background(), "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Copy same path = %v, want storage.ErrSameObject", err)
	}
	if err := s.Rename(context.Background(), "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Rename same path = %v, want storage.ErrSameObject", err)
	}
}

func TestCopyPostsToCopyEndpoint(t *testing.T) {
	t.Parallel()
	var gotPath string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Copy(context.Background(), "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if gotPath != "/storage/v1/object/copy" {
		t.Errorf("path = %s", gotPath)
	}
	if body["bucketId"] != "bucket" || body["sourceKey"] != "src.txt" || body["destinationKey"] != "dst.txt" {
		t.Errorf("body = %#v", body)
	}
}

func TestRenamePostsToMoveEndpoint(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Rename(context.Background(), "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if gotPath != "/storage/v1/object/move" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestObjectOpFailureErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Copy(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected copy error")
	}
}

func TestEscapeObjectPathEncodesDotSegments(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"a/./b":   "a/%2E/b",
		"a/../b":  "a/%2E%2E/b",
		"../x":    "%2E%2E/x",
		"a/b.txt": "a/b.txt",
		"a b":     "a%20b",
	}
	for in, want := range cases {
		if got := escapeObjectPath(in); got != want {
			t.Errorf("escapeObjectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestObjectPathsAreEscaped(t *testing.T) {
	t.Parallel()

	const key = "dir/a b?admin=1#frag.txt"
	var gotPath, gotQuery, gotFragment string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotFragment = r.URL.Path, r.URL.RawQuery, r.URL.Fragment
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"a.txt","metadata":{"size":1}}`))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.Head(context.Background(), key); err != nil {
		t.Fatalf("Head: %v", err)
	}
	if want := "/storage/v1/object/info/bucket/" + key; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotQuery != "" || gotFragment != "" {
		t.Errorf("key injected query %q / fragment %q into the request target", gotQuery, gotFragment)
	}

	url, err := s.URL(context.Background(), key)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if strings.Contains(url, "?") || strings.Contains(url, "#") {
		t.Errorf("public URL = %q, want the key percent-encoded", url)
	}
}
