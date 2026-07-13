package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/storage"
)

// newTestStorage builds an S3 Storage whose client targets the given fake S3
// endpoint using path-style addressing.
func newTestStorage(t *testing.T, endpoint string) *Storage {
	t.Helper()
	cfg := &Config{
		Bucket:         "bucket",
		Region:         DefaultRegion,
		Endpoint:       endpoint,
		AccessKey:      "AKIA_TEST",
		SecretKey:      "secret",
		ForcePathStyle: true,
	}
	s, err := NewStorage(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	return s
}

func TestUploadDownloadDeleteExists(t *testing.T) {
	t.Parallel()
	objects := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			objects[key] = string(b)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, body)
		case http.MethodHead:
			if _, ok := objects[key]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	ctx := context.Background()

	if err := s.Upload(ctx, "dir/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	ok, err := s.Exists(ctx, "dir/a.txt")
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
	rc, err := s.Download(ctx, "dir/a.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("body = %q", got)
	}
	if err := s.Delete(ctx, "dir/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, err = s.Exists(ctx, "dir/a.txt")
	if err != nil || ok {
		t.Fatalf("Exists after delete = %v, %v", ok, err)
	}
}

func TestDownloadError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if _, err := s.Download(context.Background(), "x"); err == nil {
		t.Fatal("expected download error")
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>bucket</Name>
  <Prefix>dir/</Prefix>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>dir/a.txt</Key><Size>5</Size><LastModified>2023-01-01T00:00:00.000Z</LastModified></Contents>
  <Contents><Key>dir/b.txt</Key><Size>7</Size><LastModified>2023-01-02T00:00:00.000Z</LastModified></Contents>
</ListBucketResult>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "2" {
			t.Errorf("expected ListObjectsV2, query=%s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	files, err := s.List(context.Background(), "dir/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 || files[0].Path != "dir/a.txt" || files[0].Size != 5 {
		t.Fatalf("files = %#v", files)
	}
	if files[0].LastModified.IsZero() {
		t.Fatal("expected LastModified")
	}
}

func TestListError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if _, err := s.List(context.Background(), "dir/"); err == nil {
		t.Fatal("expected list error")
	}
}

func TestUploadError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if err := s.Upload(context.Background(), "x", strings.NewReader("y")); err == nil {
		t.Fatal("expected upload error")
	}
}

func TestExistsFalseOnError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	ok, err := s.Exists(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
}

func TestURLCustomEndpoint(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, "https://minio.example:9000")
	got, err := s.URL(context.Background(), "dir/a.txt")
	if err != nil || got != "https://minio.example:9000/bucket/dir/a.txt" {
		t.Fatalf("URL = %q, %v", got, err)
	}
}

func TestURLDefaultEndpoint(t *testing.T) {
	t.Parallel()
	cfg := &Config{Bucket: "bucket", Region: "eu-west-1"}
	s, err := NewStorage(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	got, _ := s.URL(context.Background(), "a.txt")
	if got != "https://s3.eu-west-1.amazonaws.com/bucket/a.txt" {
		t.Fatalf("URL = %q", got)
	}
}

func TestSignedURL(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, "https://minio.example:9000")
	got, err := s.SignedURL(context.Background(), "dir/a.txt", 15*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if !strings.Contains(got, "/bucket/dir/a.txt") || !strings.Contains(got, "X-Amz-Signature") {
		t.Fatalf("signed url = %q", got)
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{Bucket: "bucket"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get(storage.ProviderS3); !ok {
		t.Fatal("s3 provider missing")
	}
}

func TestRegisterRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	if err := Register(storage.NewFactoryRegistry(), Config{}); err == nil {
		t.Fatal("expected invalid config error")
	}
}

func TestConfigDefaultsAndValidate(t *testing.T) {
	t.Parallel()
	c := Config{Bucket: "b"}
	c.ApplyDefaults()
	if c.Region != DefaultRegion {
		t.Fatalf("Region = %q", c.Region)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if c.GetBucket() != "b" {
		t.Fatalf("GetBucket = %q", c.GetBucket())
	}
	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("expected invalid config")
	}
}

func TestRegisterFactoryConstructs(t *testing.T) {
	t.Parallel()
	reg := storage.NewFactoryRegistry()
	if err := Register(reg, Config{Bucket: "bucket"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	f, ok := reg.Get(storage.ProviderS3)
	if !ok {
		t.Fatal("s3 provider missing")
	}
	s, err := f(storage.Config{}, (*logging.Logger)(nil))
	if err != nil || s == nil {
		t.Fatalf("factory = %v, %v", s, err)
	}
}

func TestNewStorage_ForcePathStyleWithoutEndpoint(t *testing.T) {
	t.Parallel()
	s, err := NewStorage(context.Background(), &Config{
		Bucket:         "bucket",
		Region:         DefaultRegion,
		ForcePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if !s.client.Options().UsePathStyle {
		t.Fatal("expected path-style addressing enabled")
	}
}

func TestDelete_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code></Error>`)
	}))
	defer srv.Close()
	s := newTestStorage(t, srv.URL)
	if err := s.Delete(context.Background(), "x"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestList_Paginated(t *testing.T) {
	t.Parallel()
	page1 := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>bucket</Name><IsTruncated>true</IsTruncated>
  <NextContinuationToken>tok</NextContinuationToken>
  <Contents><Key>a.txt</Key><Size>1</Size></Contents>
</ListBucketResult>`
	page2 := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>bucket</Name><IsTruncated>false</IsTruncated>
  <Contents><Key>b.txt</Key><Size>2</Size></Contents>
</ListBucketResult>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("continuation-token") == "tok" {
			_, _ = io.WriteString(w, page2)
			return
		}
		_, _ = io.WriteString(w, page1)
	}))
	defer srv.Close()
	s := newTestStorage(t, srv.URL)
	files, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files across pages, got %d", len(files))
	}
}
