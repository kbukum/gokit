package supabase

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/storage"
)

const testSecret = "service-role-secret"

// newTestStorage returns a Storage whose HTTP client targets srv, plus the
// config used to build it.
func newTestStorage(t *testing.T, srv *httptest.Server) *Storage {
	t.Helper()
	s, err := NewStorage(&Config{
		URL:       srv.URL,
		Bucket:    "bucket",
		SecretKey: testSecret,
	})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	s.httpClient = srv.Client()
	return s
}

// assertHeaderOnlyToken verifies the secret is sent via Authorization header and
// never leaks into the request URL/query string.
func assertHeaderOnlyToken(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+testSecret {
		t.Fatalf("Authorization header = %q, want bearer secret", got)
	}
	if strings.Contains(r.URL.RawQuery, testSecret) || strings.Contains(r.URL.Path, testSecret) {
		t.Fatalf("secret leaked into URL: %s", r.URL.String())
	}
}

func TestUploadSendsTokenHeaderOnly(t *testing.T) {
	t.Parallel()
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/storage/v1/object/bucket/dir/a.txt" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-upsert") != "true" {
			t.Errorf("missing x-upsert header")
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Upload(context.Background(), "dir/a.txt", strings.NewReader("payload")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotBody != "payload" {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDownloadReturnsBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	rc, err := s.Download(context.Background(), "dir/a.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(got) != "content" {
		t.Fatalf("body = %q", got)
	}
}

func TestDownloadNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.Download(context.Background(), "missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDownloadServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.Download(context.Background(), "x"); err == nil {
		t.Fatal("expected server error")
	}
}

func TestDeleteAndExists(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	ctx := context.Background()
	if err := s.Delete(ctx, "dir/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, err := s.Exists(ctx, "dir/a.txt")
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
}

func TestDeleteIgnoresNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete not-found should be nil, got %v", err)
	}
}

func TestDeleteServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Delete(context.Background(), "x"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestExistsNotFoundAndError(t *testing.T) {
	t.Parallel()
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	ok, err := s.Exists(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("Exists not-found = %v, %v", ok, err)
	}
	status = http.StatusInternalServerError
	if _, err := s.Exists(context.Background(), "x"); err == nil {
		t.Fatal("expected exists error")
	}
}

func TestURLIsPublicPath(t *testing.T) {
	t.Parallel()
	s, err := NewStorage(&Config{URL: "https://proj.supabase.co", Bucket: "bucket", SecretKey: testSecret})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	got, _ := s.URL(context.Background(), "dir/a.txt")
	want := "https://proj.supabase.co/storage/v1/object/public/bucket/dir/a.txt"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"a.txt","metadata":{"size":5,"mimetype":"text/plain"},"updated_at":"2023-01-01T00:00:00Z"}]`))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	files, err := s.List(context.Background(), "dir/a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Path != "dir/a.txt" || files[0].Size != 5 {
		t.Fatalf("files = %#v", files)
	}
	if files[0].LastModified.IsZero() {
		t.Fatal("expected parsed LastModified")
	}
}

func TestListServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.List(context.Background(), ""); err == nil {
		t.Fatal("expected list error")
	}
}

func TestSignedURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaderOnlyToken(t, r)
		_, _ = w.Write([]byte(`{"signedURL":"/object/sign/bucket/a.txt?token=abc"}`))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	got, err := s.SignedURL(context.Background(), "a.txt", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if !strings.HasSuffix(got, "/object/sign/bucket/a.txt?token=abc") {
		t.Fatalf("signed url = %q", got)
	}
}

func TestSignedURLAbsolute(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"signedURL":"https://cdn.example/a.txt?token=abc"}`))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	got, err := s.SignedURL(context.Background(), "a.txt", time.Minute)
	if err != nil || got != "https://cdn.example/a.txt?token=abc" {
		t.Fatalf("signed url = %q, %v", got, err)
	}
}

func TestSignedURLEmptyAndError(t *testing.T) {
	t.Parallel()
	empty := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if empty {
			_, _ = w.Write([]byte(`{"signedURL":""}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if _, err := s.SignedURL(context.Background(), "a.txt", time.Minute); err == nil {
		t.Fatal("expected empty-url error")
	}
	empty = false
	if _, err := s.SignedURL(context.Background(), "a.txt", time.Minute); err == nil {
		t.Fatal("expected server error")
	}
}

func TestUploadServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer srv.Close()

	s := newTestStorage(t, srv)
	if err := s.Upload(context.Background(), "x", strings.NewReader("y")); err == nil {
		t.Fatal("expected upload error")
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{URL: "https://p.supabase.co", Bucket: "b", SecretKey: "s"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get(storage.ProviderSupabase); !ok {
		t.Fatal("supabase provider missing")
	}
}

func TestRegisterRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	if err := Register(storage.NewFactoryRegistry(), Config{}); err == nil {
		t.Fatal("expected invalid config error")
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()
	if err := Register(nil, Config{URL: "https://p.supabase.co", Bucket: "b", SecretKey: "s"}); err == nil {
		t.Fatal("expected nil registry error")
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	c := Config{URL: "u", Bucket: "b", SecretKey: "s"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if c.GetBucket() != "b" {
		t.Fatalf("GetBucket = %q", c.GetBucket())
	}
	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("expected missing-field error")
	}
}

func TestFromSupabase(t *testing.T) {
	t.Parallel()
	if FromSupabase(nil) != nil {
		t.Fatal("nil should map to nil")
	}
	cases := []string{
		"Invalid login credentials",
		"Email not confirmed",
		"JWT expired",
		"invalid jwt",
		"rate limit exceeded",
		"User already registered",
		"password is too weak",
		"invalid email format",
		"connection refused",
		"user not found",
		"permission denied",
		"something unexpected",
	}
	for _, msg := range cases {
		if got := FromSupabase(errors.New(msg)); got == nil {
			t.Fatalf("FromSupabase(%q) = nil", msg)
		}
	}
}

// errTransport fails every round trip, exercising httpClient.Do error paths.
type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func newErrStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(&Config{URL: "https://p.supabase.co", Bucket: "b", SecretKey: "k"})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	s.httpClient = &http.Client{Transport: errTransport{}}
	return s
}

func TestAllOperationsWrapTransportError(t *testing.T) {
	t.Parallel()
	s := newErrStorage(t)
	ctx := context.Background()
	if err := s.Upload(ctx, "a", strings.NewReader("x")); err == nil {
		t.Error("Upload should error")
	}
	if _, err := s.Download(ctx, "a"); err == nil {
		t.Error("Download should error")
	}
	if err := s.Delete(ctx, "a"); err == nil {
		t.Error("Delete should error")
	}
	if _, err := s.Exists(ctx, "a"); err == nil {
		t.Error("Exists should error")
	}
	if _, err := s.List(ctx, "a"); err == nil {
		t.Error("List should error")
	}
	if _, err := s.SignedURL(ctx, "a", time.Minute); err == nil {
		t.Error("SignedURL should error")
	}
}

func TestListDecodeErrorAndSort(t *testing.T) {
	t.Parallel()
	// Bad JSON => decode error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer bad.Close()
	sbad := newTestStorage(t, bad)
	if _, err := sbad.List(context.Background(), ""); err == nil {
		t.Fatal("expected decode error")
	}

	// Two files => exercises the sort comparator; prefix without slash hits the
	// search-only branch.
	multi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"b.txt","metadata":{"size":1}},{"name":"a.txt","metadata":{"size":2}}]`))
	}))
	defer multi.Close()
	s := newTestStorage(t, multi)
	files, err := s.List(context.Background(), "a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 || files[0].Path != "a.txt" {
		t.Fatalf("sorted files = %#v", files)
	}
}

func TestSignedURLDecodeError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	s := newTestStorage(t, srv)
	if _, err := s.SignedURL(context.Background(), "a.txt", time.Minute); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestRegisterFactoryConstructsStorage(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{URL: "https://p.supabase.co", Bucket: "b", SecretKey: "s"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	f, ok := reg.Get(storage.ProviderSupabase)
	if !ok {
		t.Fatal("provider missing")
	}
	got, err := f(storage.Config{}, (*logging.Logger)(nil))
	if err != nil || got == nil {
		t.Fatalf("factory = %v, %v", got, err)
	}
}

func TestApplyDefaultsNoop(t *testing.T) {
	t.Parallel()
	c := Config{}
	c.ApplyDefaults() // documented no-op; call for coverage + contract lock
}
