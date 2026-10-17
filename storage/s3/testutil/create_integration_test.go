//go:build integration

package testutil_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/storage/s3"
	"github.com/kbukum/gokit/storage/s3/testutil"
	storagetest "github.com/kbukum/gokit/storage/testutil"
)

func TestCreateRefusesAnExistingObjectOnTheRealServer(t *testing.T) {
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
		if err := store.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	if err := storage.Create(t.Context(), store, "fenced/key", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Create(t.Context(), store, "fenced/key", strings.NewReader("second")); !errors.Is(err, storage.ErrExists) {
		t.Fatalf("create replaced an existing object: %v", err)
	}
	if err := storage.Create(t.Context(), store, "fenced/empty", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Create(t.Context(), store, "fenced/empty", strings.NewReader("late")); !errors.Is(err, storage.ErrExists) {
		t.Fatalf("an empty object did not fence its key: %v", err)
	}
	reader, err := store.Download(t.Context(), "fenced/key")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close()); err != nil || string(data) != "first" {
		t.Fatalf("refused create changed the stored bytes: %q, %v", data, err)
	}
	if err := store.Delete(t.Context(), "fenced/key"); err != nil {
		t.Fatal(err)
	}
	if err := storage.Create(t.Context(), store, "fenced/key", strings.NewReader("third")); err != nil {
		t.Fatalf("create after deletion was refused: %v", err)
	}
}

func TestS3StorageConformance(t *testing.T) {
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
		if err := store.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	storagetest.Conformance(t, func(*testing.T) storage.Storage { return store })
}
