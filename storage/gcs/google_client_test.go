package gcs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gcstorage "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

func TestGoogleClientMethodsUseStorageJSONAPI(t *testing.T) {
	t.Parallel()
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		defer r.Body.Close()
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/objects/o"):
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"dir/a.txt","bucket":"objects"}`))
		case r.Method == http.MethodGet && (r.URL.Query().Get("alt") == "media" || r.URL.Path == "/objects/dir/a.txt" || r.URL.Path == "/objects/dir%2Fa.txt"):
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodGet && (strings.Contains(r.URL.Path, "/storage/v1/b/objects/o/dir%2Fa.txt") || strings.Contains(r.URL.Path, "/storage/v1/b/objects/o/dir/a.txt")) || r.URL.Path == "/objects/dir/a.txt" || r.URL.Path == "/objects/dir%2Fa.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"dir/a.txt","bucket":"objects","size":"5","updated":"2026-07-12T00:00:00Z","contentType":"text/plain"}`))
		case (r.Method == http.MethodGet && r.URL.Path == "/storage/v1/b/objects/o") || r.URL.Path == "/objects":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"name":"dir/a.txt","bucket":"objects","size":"5","updated":"2026-07-12T00:00:00Z","contentType":"text/plain"}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.RequestURI()})
		}
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := gcstorage.NewClient(ctx, option.WithEndpoint(server.URL+"/storage/v1/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	gc := &googleClient{bucket: "objects", client: client, cfg: &Config{Bucket: "objects"}}
	if err := gc.Put(ctx, "dir/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	body, err := gc.Get(ctx, "dir/a.txt")
	if err != nil {
		t.Fatalf("Get: %v requests=%v", err, requests)
	}
	data, err := io.ReadAll(body)
	if closeErr := body.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if err != nil || string(data) != "hello" {
		t.Fatalf("body = %q, %v", data, err)
	}
	exists, err := gc.Exists(ctx, "dir/a.txt")
	if err != nil || !exists {
		t.Fatalf("Exists = %v, %v", exists, err)
	}
	files, err := gc.List(ctx, "dir/")
	if err != nil || len(files) != 1 || files[0].Path != "dir/a.txt" || files[0].Size != 5 {
		t.Fatalf("List = %#v, %v", files, err)
	}
	if err := gc.Delete(ctx, "dir/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestGoogleClientSignedURLRequiresSigningConfig(t *testing.T) {
	t.Parallel()
	gc := &googleClient{bucket: "objects", cfg: &Config{Bucket: "objects"}}
	if _, err := gc.SignedURL(context.Background(), "a.txt", time.Minute); err == nil {
		t.Fatal("expected missing signing config error")
	}
}

func TestGoogleClientSignedURLWithSigningConfig(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	gc := &googleClient{bucket: "objects", cfg: &Config{Bucket: "objects", GoogleAccessID: "svc@example.test", PrivateKey: pemKey}}
	signed, err := gc.SignedURL(context.Background(), "a.txt", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if !strings.Contains(signed, "GoogleAccessId=svc%40example.test") {
		t.Fatalf("signed URL missing access id: %s", signed)
	}
}

// errReader always fails, exercising the io.Copy error path in Put.
type gcErrReader struct{}

func (gcErrReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestGoogleClientPutReaderError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, err := gcstorage.NewClient(ctx,
		option.WithEndpoint("http://127.0.0.1:0/storage/v1/"),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	gc := &googleClient{bucket: "objects", client: client, cfg: &Config{Bucket: "objects"}}
	if err := gc.Put(ctx, "a.txt", gcErrReader{}); err == nil {
		t.Fatal("expected put error from failing reader")
	}
}

func TestGoogleClientMethodsPropagateServerErrors(t *testing.T) {
	t.Parallel()
	// Return a non-retryable status so the SDK surfaces the error immediately
	// instead of retrying 5xx responses with backoff.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := gcstorage.NewClient(ctx,
		option.WithEndpoint(server.URL+"/storage/v1/"),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	gc := &googleClient{bucket: "objects", client: client, cfg: &Config{Bucket: "objects"}}

	if _, err := gc.Get(ctx, "a.txt"); err == nil {
		t.Error("Get should propagate server error")
	}
	if _, err := gc.Exists(ctx, "a.txt"); err == nil {
		t.Error("Exists should propagate server error")
	}
	if _, err := gc.List(ctx, "dir/"); err == nil {
		t.Error("List should propagate server error")
	}
	if err := gc.Delete(ctx, "a.txt"); err == nil {
		t.Error("Delete should propagate server error")
	}
}

func TestNewGoogleClientAppliesCredentialOptions(t *testing.T) {
	t.Parallel()
	// A nonexistent credentials file makes NewClient fail after the endpoint and
	// credentials-file options are applied, exercising those branches.
	_, err := newGoogleClient(context.Background(), &Config{
		Bucket:          "objects",
		Endpoint:        "http://127.0.0.1:0",
		CredentialsFile: "/nonexistent/creds.json",
	})
	if err == nil {
		t.Fatal("expected error for missing credentials file")
	}
}

func TestNewGoogleClientRejectsInvalidCredentialsJSON(t *testing.T) {
	t.Parallel()
	_, err := newGoogleClient(context.Background(), &Config{
		Bucket:          "objects",
		CredentialsJSON: []byte("not json"),
	})
	if err == nil {
		t.Fatal("expected error for invalid credentials JSON")
	}
}

func TestNewStoragePropagatesClientError(t *testing.T) {
	t.Parallel()
	_, err := NewStorage(context.Background(), &Config{
		Bucket:          "objects",
		CredentialsFile: "/nonexistent/creds.json",
	})
	if err == nil {
		t.Fatal("expected NewStorage error when client construction fails")
	}
}

func TestGoogleClientTreatsMissingObjectAsAbsent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := gcstorage.NewClient(ctx,
		option.WithEndpoint(server.URL+"/storage/v1/"),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	gc := &googleClient{bucket: "objects", client: client, cfg: &Config{Bucket: "objects"}}

	if err := gc.Delete(ctx, "missing.txt"); err != nil {
		t.Errorf("Delete of missing object should be nil, got %v", err)
	}
	exists, err := gc.Exists(ctx, "missing.txt")
	if err != nil {
		t.Errorf("Exists error: %v", err)
	}
	if exists {
		t.Error("Exists should report false for missing object")
	}
}

func TestNewGoogleClientUsesProvidedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	creds := []byte(`{"type":"authorized_user","client_id":"id","client_secret":"secret","refresh_token":"token"}`)
	gc, err := newGoogleClient(context.Background(), &Config{
		Bucket:          "objects",
		Endpoint:        server.URL,
		CredentialsJSON: creds,
	})
	if err != nil {
		t.Fatalf("newGoogleClient: %v", err)
	}
	if gc == nil {
		t.Fatal("expected client")
	}
}
