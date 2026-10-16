//go:build integration

package testutil_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/storage/s3"
	"github.com/kbukum/gokit/storage/s3/testutil"
)

func TestS3FixturePersistsAcrossRestartAndRefusesUnprivilegedSetup(t *testing.T) {
	t.Parallel()
	f, err := testutil.StartS3(t.Context(), t.TempDir(), testutil.IntegrationOptions(t)...)
	if f != nil {
		t.Cleanup(func() {
			if err := f.Close(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.Config()
	store, err := s3.NewStorage(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	if err := store.Upload(t.Context(), "owned/final", strings.NewReader("durable")); err != nil {
		t.Fatal(err)
	}
	if err := store.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.CrashRestart(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg = f.Config()
	restarted, err := s3.NewStorage(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restarted.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	reader, err := restarted.Download(t.Context(), "owned/final")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if readErr != nil || string(data) != "durable" {
		t.Fatalf("restart lost acknowledged bytes: %v", readErr)
	}
	cfg.Bucket = "unauthorized-bucket"
	other, err := s3.NewStorage(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := other.Close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	if err := other.Upload(t.Context(), "object", strings.NewReader("must fail")); err == nil {
		t.Fatal("runtime identity wrote outside its provisioned bucket")
	}
	if err := other.Check(t.Context()); err == nil {
		t.Fatal("runtime identity reported an unauthorized bucket ready")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.Config().Endpoint+"/"+f.Config().Bucket+"/owned/final", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Timeout: 2 * time.Second}
	defer httpClient.CloseIdleConnections()
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymous content read status = %d", resp.StatusCode)
	}
	if err := f.Restart(t.Context()); err != nil {
		t.Fatal(err)
	}
}
