package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/storage"
)

// newObjectServer returns a minimal S3-compatible fake supporting PUT (including
// server-side copy), GET, HEAD, and DELETE against an in-memory object map.
func newObjectServer(objects map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		switch r.Method {
		case http.MethodPut:
			if src := r.Header.Get("x-amz-copy-source"); src != "" {
				srcKey := strings.TrimPrefix(src, "bucket/")
				body, ok := objects[srcKey]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				objects[key] = body
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<CopyObjectResult><ETag>"abc"</ETag></CopyObjectResult>`)
				return
			}
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
			body, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"9a0364b9e99bb480dd25e1f0284c8555"`)
			w.Header().Set("x-amz-checksum-sha256", "3q2+7w==")
			w.Header().Set("x-amz-meta-owner", "team")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func TestHead_ReturnsMetadataAndChecksum(t *testing.T) {
	t.Parallel()
	objects := map[string]string{"dir/a.txt": "content"}
	srv := newObjectServer(objects)
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	info, err := s.Head(context.Background(), "dir/a.txt")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != int64(len("content")) {
		t.Errorf("Size = %d", info.Size)
	}
	if info.ContentType != "text/plain" {
		t.Errorf("ContentType = %q", info.ContentType)
	}
	if info.Checksum != "sha256:3q2+7w==" {
		t.Errorf("Checksum = %q, want the explicit S3 content checksum, not the ETag", info.Checksum)
	}
	if info.Metadata["owner"] != "team" {
		t.Errorf("Metadata = %#v", info.Metadata)
	}
}

func TestHead_MissingErrors(t *testing.T) {
	t.Parallel()
	srv := newObjectServer(map[string]string{})
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	_, err := s.Head(context.Background(), "absent")
	if err == nil {
		t.Fatal("Head for a missing object did not error")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Head error is not storage.ErrNotFound: %v", err)
	}
}

func TestCopyRename_RejectSamePath(t *testing.T) {
	t.Parallel()
	srv := newObjectServer(map[string]string{"a.txt": "payload"})
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if err := s.Copy(context.Background(), "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Copy same path = %v, want storage.ErrSameObject", err)
	}
	if err := s.Rename(context.Background(), "a.txt", "a.txt"); !errors.Is(err, storage.ErrSameObject) {
		t.Errorf("Rename same path = %v, want storage.ErrSameObject", err)
	}
}

func TestCopy_DuplicatesObject(t *testing.T) {
	t.Parallel()
	objects := map[string]string{"src.txt": "payload"}
	srv := newObjectServer(objects)
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if err := s.Copy(context.Background(), "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if objects["dst.txt"] != "payload" {
		t.Fatalf("dst = %q", objects["dst.txt"])
	}
	if objects["src.txt"] != "payload" {
		t.Fatalf("src removed: %q", objects["src.txt"])
	}
}

func TestRename_MovesObject(t *testing.T) {
	t.Parallel()
	objects := map[string]string{"src.txt": "payload"}
	srv := newObjectServer(objects)
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if err := s.Rename(context.Background(), "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if objects["dst.txt"] != "payload" {
		t.Fatalf("dst = %q", objects["dst.txt"])
	}
	if _, ok := objects["src.txt"]; ok {
		t.Fatal("source still present after rename")
	}
}

func TestHead_EnablesChecksumMode(t *testing.T) {
	t.Parallel()
	modeCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			select {
			case modeCh <- r.Header.Get("x-amz-checksum-mode"):
			default:
			}
			w.Header().Set("Content-Length", "7")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	if _, err := s.Head(context.Background(), "dir/a.txt"); err != nil {
		t.Fatalf("Head: %v", err)
	}
	select {
	case mode := <-modeCh:
		if mode != "ENABLED" {
			t.Fatalf("HeadObject x-amz-checksum-mode = %q, want ENABLED", mode)
		}
	default:
		t.Fatal("HeadObject request was not observed")
	}
}

func TestDownload_MissingReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	srv := newObjectServer(map[string]string{})
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	_, err := s.Download(context.Background(), "absent")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Download of a missing object = %v, want storage.ErrNotFound", err)
	}
}

func TestCopy_MissingSourceReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	srv := newObjectServer(map[string]string{})
	defer srv.Close()

	s := newTestStorage(t, srv.URL)
	err := s.Copy(context.Background(), "absent", "dst.txt")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Copy of a missing source = %v, want storage.ErrNotFound", err)
	}
}

func TestConfig_UsesAWSCredentialKeys(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Bucket:          "bucket",
		Region:          DefaultRegion,
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "secret",
	}
	if _, err := NewStorage(context.Background(), cfg); err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
}
