package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
	gostorage "github.com/kbukum/gokit/storage"
)

// fakeStorage is an in-memory storage.Storage with fault injection for tests.
type fakeStorage struct {
	objects   map[string][]byte
	uploadErr error
	failList  bool
	// downloadErr maps a path to an error returned by Download.
	downloadErr map[string]error
}

var _ gostorage.Storage = (*fakeStorage)(nil)

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		objects:     make(map[string][]byte),
		downloadErr: make(map[string]error),
	}
}

func (f *fakeStorage) Upload(_ context.Context, path string, reader io.Reader) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.objects[path] = data
	return nil
}

func (f *fakeStorage) Download(_ context.Context, path string) (io.ReadCloser, error) {
	if err := f.downloadErr[path]; err != nil {
		return nil, err
	}
	data, ok := f.objects[path]
	if !ok {
		return nil, errors.New("not found: " + path)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeStorage) Delete(_ context.Context, path string) error {
	delete(f.objects, path)
	return nil
}

func (f *fakeStorage) Exists(_ context.Context, path string) (bool, error) {
	_, ok := f.objects[path]
	return ok, nil
}

func (f *fakeStorage) URL(_ context.Context, path string) (string, error) {
	return "mem://" + path, nil
}

func (f *fakeStorage) List(_ context.Context, prefix string) ([]gostorage.FileInfo, error) {
	if f.failList {
		return nil, errors.New("list failed")
	}
	var out []gostorage.FileInfo
	for path := range f.objects {
		if strings.HasPrefix(path, prefix) {
			out = append(out, gostorage.FileInfo{Path: path})
		}
	}
	return out, nil
}

func sampleResult(id, tag, dataset string, ts time.Time, f1 float64) *bench.RunResult {
	return &bench.RunResult{
		ID:        id,
		Timestamp: ts,
		Tag:       tag,
		Dataset:   bench.DatasetInfo{Name: dataset},
		Metrics: []bench.MetricResult{
			{Name: "classification", Values: map[string]float64{"f1": f1}},
		},
	}
}

func TestNewProviderStorageDefaultPrefix(t *testing.T) {
	t.Parallel()
	s := NewProviderStorage(newFakeStorage())
	if got := s.key("run1"); got != "bench/run1.json" {
		t.Fatalf("key = %q, want bench/run1.json", got)
	}
}

func TestWithPrefix(t *testing.T) {
	t.Parallel()
	s := NewProviderStorage(newFakeStorage(), WithPrefix("results/"))
	if got := s.key("run1"); got != "results/run1.json" {
		t.Fatalf("key = %q, want results/run1.json", got)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeStorage()
	s := NewProviderStorage(fake)

	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	want := sampleResult("run-a", "nightly", "ds1", ts, 0.9)

	id, err := s.Save(ctx, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id != "run-a" {
		t.Fatalf("Save id = %q, want run-a", id)
	}

	got, err := s.Load(ctx, "run-a")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != want.ID || got.Tag != want.Tag || got.Dataset.Name != want.Dataset.Name {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveMarshalError(t *testing.T) {
	t.Parallel()
	s := NewProviderStorage(newFakeStorage())
	// Curves holds a channel, which encoding/json cannot marshal.
	bad := sampleResult("run-x", "", "ds", time.Now(), 0)
	bad.Curves = map[string]any{"c": make(chan int)}

	if _, err := s.Save(context.Background(), bad); err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestSaveUploadError(t *testing.T) {
	t.Parallel()
	fake := newFakeStorage()
	fake.uploadErr = errors.New("boom")
	s := NewProviderStorage(fake)

	if _, err := s.Save(context.Background(), sampleResult("r", "", "d", time.Now(), 0)); err == nil {
		t.Fatal("expected upload error, got nil")
	}
}

func TestLoadDownloadError(t *testing.T) {
	t.Parallel()
	s := NewProviderStorage(newFakeStorage())
	if _, err := s.Load(context.Background(), "missing"); err == nil {
		t.Fatal("expected download error, got nil")
	}
}

func TestLoadDecodeError(t *testing.T) {
	t.Parallel()
	fake := newFakeStorage()
	fake.objects["bench/broken.json"] = []byte("{not json")
	s := NewProviderStorage(fake)

	if _, err := s.Load(context.Background(), "broken"); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestLatestEmpty(t *testing.T) {
	t.Parallel()
	s := NewProviderStorage(newFakeStorage())
	if _, err := s.Latest(context.Background()); err == nil {
		t.Fatal("expected error for empty storage, got nil")
	}
}

func TestLatestReturnsMostRecent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeStorage()
	s := NewProviderStorage(fake)

	older := sampleResult("old", "", "ds", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 0.5)
	newer := sampleResult("new", "", "ds", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), 0.8)
	if _, err := s.Save(ctx, older); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, newer); err != nil {
		t.Fatal(err)
	}

	got, err := s.Latest(ctx)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.ID != "new" {
		t.Fatalf("Latest ID = %q, want new", got.ID)
	}
}

func TestListError(t *testing.T) {
	t.Parallel()
	fake := newFakeStorage()
	fake.failList = true
	s := NewProviderStorage(fake)

	if _, err := s.List(context.Background()); err == nil {
		t.Fatal("expected list error, got nil")
	}
}

func TestListSortsFiltersAndLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeStorage()
	s := NewProviderStorage(fake)

	mustSave := func(r *bench.RunResult) {
		if _, err := s.Save(ctx, r); err != nil {
			t.Fatalf("Save %s: %v", r.ID, err)
		}
	}
	mustSave(sampleResult("a", "nightly", "ds1", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 0.1))
	mustSave(sampleResult("b", "nightly", "ds1", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), 0.2))
	mustSave(sampleResult("c", "release", "ds2", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), 0.3))

	// Non-JSON and unrelated objects must be ignored.
	fake.objects["bench/notes.txt"] = []byte("ignore me")

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List len = %d, want 3", len(all))
	}
	if all[0].ID != "b" || all[1].ID != "c" || all[2].ID != "a" {
		t.Fatalf("List order = %v, want [b c a] (timestamp desc)", []string{all[0].ID, all[1].ID, all[2].ID})
	}

	tagged, err := s.List(ctx, bench.WithTagFilter("release"))
	if err != nil {
		t.Fatalf("List tag: %v", err)
	}
	if len(tagged) != 1 || tagged[0].ID != "c" {
		t.Fatalf("tag filter = %v, want [c]", tagged)
	}

	byDataset, err := s.List(ctx, bench.WithDatasetFilter("ds1"))
	if err != nil {
		t.Fatalf("List dataset: %v", err)
	}
	if len(byDataset) != 2 {
		t.Fatalf("dataset filter len = %d, want 2", len(byDataset))
	}

	limited, err := s.List(ctx, bench.WithLimit(1))
	if err != nil {
		t.Fatalf("List limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "b" {
		t.Fatalf("limit = %v, want [b]", limited)
	}
}

func TestListSkipsUndecodableAndDownloadErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeStorage()
	s := NewProviderStorage(fake)

	if _, err := s.Save(ctx, sampleResult("good", "", "ds", time.Now(), 0.4)); err != nil {
		t.Fatal(err)
	}
	fake.objects["bench/corrupt.json"] = []byte("{bad")
	fake.objects["bench/unreadable.json"] = []byte("{}")
	fake.downloadErr["bench/unreadable.json"] = errors.New("io error")

	got, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("List = %v, want only [good]", got)
	}
}

func TestListF1FromMetricValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeStorage()
	s := NewProviderStorage(fake)

	r := &bench.RunResult{
		ID:        "mc",
		Timestamp: time.Now(),
		Dataset:   bench.DatasetInfo{Name: "ds"},
		Metrics: []bench.MetricResult{
			{Name: "multi_class_classification", Value: 0.77},
		},
	}
	if _, err := s.Save(ctx, r); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].F1 != 0.77 {
		t.Fatalf("F1 = %v, want 0.77", got)
	}
}
