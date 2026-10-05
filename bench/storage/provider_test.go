package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/bench"
	gostorage "github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/storage/local"
)

type faultStore struct {
	gostorage.Storage
	uploadErr, downloadErr, listErr error
	reader                          io.ReadCloser
	files                           []gostorage.FileInfo
}

func (s *faultStore) Upload(ctx context.Context, key string, r io.Reader) error {
	if s.uploadErr != nil {
		return s.uploadErr
	}
	return s.Storage.Upload(ctx, key, r)
}

func (s *faultStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	if s.reader != nil {
		return s.reader, nil
	}
	return s.Storage.Download(ctx, key)
}

func (s *faultStore) List(ctx context.Context, prefix string) ([]gostorage.FileInfo, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.files != nil {
		return s.files, nil
	}
	return s.Storage.List(ctx, prefix)
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
func (r failingReader) Close() error             { return r.err }

func TestProviderObjectPort(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"bench/", "custom/"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			backend, err := local.NewStorage(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			adapter := NewProviderStorage(backend, WithPrefix(prefix))
			if err := adapter.Put(t.Context(), "records/run/0/00000000.jsonl", []byte("data")); err != nil {
				t.Fatal(err)
			}
			data, err := adapter.Get(t.Context(), "records/run/0/00000000.jsonl", 4)
			if err != nil || string(data) != "data" {
				t.Fatalf("get=%q %v", data, err)
			}
			keys, err := adapter.List(t.Context(), "records/")
			if err != nil || !reflect.DeepEqual(keys, []string{"records/run/0/00000000.jsonl"}) {
				t.Fatalf("list=%v %v", keys, err)
			}
			if err := adapter.Delete(t.Context(), keys[0]); err != nil {
				t.Fatal(err)
			}
			keys, err = adapter.List(t.Context(), "")
			if err != nil || len(keys) != 0 {
				t.Fatalf("cleanup=%v %v", keys, err)
			}
		})
	}
}

func TestProviderErrors(t *testing.T) {
	t.Parallel()
	cause := errors.New("injected")
	for _, tc := range []struct {
		name string
		run  func(*ProviderStorage) error
	}{
		{"upload", func(s *ProviderStorage) error { return s.Put(t.Context(), "key", nil) }},
		{"download", func(s *ProviderStorage) error { _, err := s.Get(t.Context(), "key", 10); return err }},
		{"list", func(s *ProviderStorage) error { _, err := s.List(t.Context(), ""); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewProviderStorage(&faultStore{uploadErr: cause, downloadErr: cause, listErr: cause})
			if err := tc.run(s); !errors.Is(err, cause) {
				t.Fatalf("cause lost: %v", err)
			}
		})
	}
	for _, key := range []string{"../escape", "/absolute", ""} {
		s := NewProviderStorage(&faultStore{})
		if err := s.Put(t.Context(), key, nil); err == nil {
			t.Fatalf("invalid key %q", key)
		}
		if _, err := s.Get(t.Context(), key, 1); err == nil {
			t.Fatalf("invalid get %q", key)
		}
		if err := s.Delete(t.Context(), key); err == nil {
			t.Fatalf("invalid delete %q", key)
		}
	}
	for _, limit := range []int64{0, 1} {
		s := NewProviderStorage(&faultStore{reader: io.NopCloser(bytes.NewReader([]byte("too big")))})
		if _, err := s.Get(t.Context(), "key", limit); err == nil {
			t.Fatal("invalid read accepted")
		}
	}
	s := NewProviderStorage(&faultStore{reader: failingReader{err: cause}})
	if _, err := s.Get(t.Context(), "key", 10); !errors.Is(err, cause) {
		t.Fatalf("read/close cause lost: %v", err)
	}
	s = NewProviderStorage(&faultStore{files: []gostorage.FileInfo{{Path: "outside/key"}}})
	if _, err := s.List(t.Context(), ""); err == nil {
		t.Fatal("out-of-namespace listing accepted")
	}
}

func TestProviderResultStoreConsumer(t *testing.T) {
	t.Parallel()
	backend, err := local.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := bench.NewResultStore(NewProviderStorage(backend))
	runner := bench.NewBenchRunner(bench.WithStore[string](store))
	runner.Register("model", bench.EvaluatorFunc("model", func(context.Context, []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "yes"}, nil
	}))
	result, err := runner.Run(t.Context(), bench.NewSliceDataset(bench.DatasetDescriptor{Name: "adapter", Version: "1"}, []bench.Sample[string]{{ID: "one", Label: "yes"}}))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(t.Context(), result.ID)
	if err != nil || loaded.ID != result.ID {
		t.Fatalf("load=%v %v", loaded, err)
	}
	if err := store.Delete(t.Context(), result.ID); err != nil {
		t.Fatal(err)
	}
}
