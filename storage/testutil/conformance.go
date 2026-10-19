package testutil

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/storage"
)

// Conformance checks the write-once guarantees a backend must keep for publication flows: Create refuses an occupied key without replacing it, an empty object fences its key, Head and Download report exactly what was written, missing objects return storage.ErrNotFound, Delete removes an object, and concurrent creates have exactly one winner.
//
// open returns the backend under test; it may be shared across subtests because each uses its own key prefix. The backend must implement storage.CreateProvider.
func Conformance(t *testing.T, open func(t *testing.T) storage.Storage) {
	t.Helper()
	prefix := func() string { return "conformance/" + strings.ToLower(rand.Text()) + "/" }
	read := func(t *testing.T, s storage.Storage, key string) string {
		t.Helper()
		r, err := s.Download(t.Context(), key)
		if err != nil {
			t.Fatalf("download %s: %v", key, err)
		}
		data, readErr := io.ReadAll(r)
		if err := errors.Join(readErr, r.Close()); err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		return string(data)
	}

	t.Run("create refuses an occupied key", func(t *testing.T) {
		s, key := open(t), prefix()+"object"
		if err := storage.Create(t.Context(), s, key, strings.NewReader("first")); err != nil {
			t.Fatal(err)
		}
		if err := storage.Create(t.Context(), s, key, strings.NewReader("second")); !errors.Is(err, storage.ErrExists) {
			t.Fatalf("occupied key was not refused: %v", err)
		}
		if got := read(t, s, key); got != "first" {
			t.Fatalf("refused create replaced the object: %q", got)
		}
	})

	t.Run("head and download report the written bytes", func(t *testing.T) {
		s, key := open(t), prefix()+"object"
		data := bytes.Repeat([]byte("payload\n"), 1024)
		if err := storage.Create(t.Context(), s, key, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		info, err := storage.Head(t.Context(), s, key)
		if err != nil || info.Size != int64(len(data)) {
			t.Fatalf("head size = %d, %v; want %d", info.Size, err, len(data))
		}
		if got := read(t, s, key); got != string(data) {
			t.Fatal("download changed the stored bytes")
		}
	})

	t.Run("empty object fences its key", func(t *testing.T) {
		s, key := open(t), prefix()+"fence"
		if err := storage.Create(t.Context(), s, key, bytes.NewReader(nil)); err != nil {
			t.Fatal(err)
		}
		if err := storage.Create(t.Context(), s, key, strings.NewReader("late")); !errors.Is(err, storage.ErrExists) {
			t.Fatalf("empty object did not fence its key: %v", err)
		}
		if info, err := storage.Head(t.Context(), s, key); err != nil || info.Size != 0 {
			t.Fatalf("fence size = %d, %v", info.Size, err)
		}
	})

	t.Run("missing objects are not found", func(t *testing.T) {
		s, key := open(t), prefix()+"missing"
		if _, err := storage.Head(t.Context(), s, key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("head of a missing object: %v", err)
		}
		if r, err := s.Download(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
			if r != nil {
				_ = r.Close()
			}
			t.Fatalf("download of a missing object: %v", err)
		}
	})

	t.Run("delete removes the object", func(t *testing.T) {
		s, key := open(t), prefix()+"object"
		if err := storage.Create(t.Context(), s, key, strings.NewReader("value")); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(t.Context(), key); err != nil {
			t.Fatal(err)
		}
		if _, err := storage.Head(t.Context(), s, key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("deleted object is still visible: %v", err)
		}
	})

	t.Run("concurrent creates have one winner", func(t *testing.T) {
		s, key := open(t), prefix()+"race"
		const writers = 8
		errs := make([]error, writers)
		var wg sync.WaitGroup
		for i := range writers {
			wg.Go(func() {
				errs[i] = storage.Create(t.Context(), s, key, strings.NewReader(fmt.Sprintf("writer-%d", i)))
			})
		}
		wg.Wait()
		winner := -1
		for i, err := range errs {
			switch {
			case err == nil && winner < 0:
				winner = i
			case err == nil:
				t.Fatalf("writers %d and %d both created the key", winner, i)
			case !errors.Is(err, storage.ErrExists):
				t.Fatalf("writer %d: %v", i, err)
			}
		}
		if winner < 0 {
			t.Fatal("no writer created the key")
		}
		if got := read(t, s, key); got != fmt.Sprintf("writer-%d", winner) {
			t.Fatalf("stored %q, want the winner's bytes", got)
		}
	})
}
