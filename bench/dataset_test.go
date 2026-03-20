package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir string, m DatasetManifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSampleFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stringMapper(s string) (string, error) { return s, nil }

func TestNewDatasetLoader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "test-ds",
		Version: "1.0",
		Samples: []ManifestSample{
			{ID: "s1", File: "s1.txt", Label: "positive"},
			{ID: "s2", File: "s2.txt", Label: "negative"},
		},
	})
	writeSampleFile(t, dir, "s1.txt", "hello world")
	writeSampleFile(t, dir, "s2.txt", "goodbye world")

	loader := NewDatasetLoader[string](dir, stringMapper)

	ctx := context.Background()
	samples, err := loader.All(ctx)
	if err != nil {
		t.Fatalf("All() error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	if samples[0].ID != "s1" {
		t.Errorf("samples[0].ID = %q, want %q", samples[0].ID, "s1")
	}
	if samples[0].Label != "positive" {
		t.Errorf("samples[0].Label = %q, want %q", samples[0].Label, "positive")
	}
	if string(samples[0].Input) != "hello world" {
		t.Errorf("samples[0].Input = %q, want %q", samples[0].Input, "hello world")
	}
}

func TestDatasetLoaderManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "my-dataset",
		Version: "2.0",
		Samples: []ManifestSample{{ID: "s1", Label: "a"}},
	})

	loader := NewDatasetLoader[string](dir, stringMapper)
	m, err := loader.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error: %v", err)
	}
	if m.Name != "my-dataset" {
		t.Errorf("Name = %q, want %q", m.Name, "my-dataset")
	}
	if m.Version != "2.0" {
		t.Errorf("Version = %q, want %q", m.Version, "2.0")
	}
}

func TestDatasetLoaderIterator(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "iter-ds",
		Version: "1.0",
		Samples: []ManifestSample{
			{ID: "s1", File: "s1.txt", Label: "a"},
			{ID: "s2", File: "s2.txt", Label: "b"},
			{ID: "s3", File: "s3.txt", Label: "c"},
		},
	})
	for _, name := range []string{"s1.txt", "s2.txt", "s3.txt"} {
		writeSampleFile(t, dir, name, "content-"+name)
	}

	loader := NewDatasetLoader[string](dir, stringMapper)
	ctx := context.Background()

	iter, err := loader.Iterator(ctx)
	if err != nil {
		t.Fatalf("Iterator() error: %v", err)
	}
	defer iter.Close()

	count := 0
	for {
		_, ok, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		if !ok {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("iterated %d samples, want 3", count)
	}
}

func TestDatasetLoaderFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "filter-ds",
		Version: "1.0",
		Samples: []ManifestSample{
			{ID: "s1", File: "s1.txt", Label: "positive"},
			{ID: "s2", File: "s2.txt", Label: "negative"},
			{ID: "s3", File: "s3.txt", Label: "positive"},
		},
	})
	for _, name := range []string{"s1.txt", "s2.txt", "s3.txt"} {
		writeSampleFile(t, dir, name, "data")
	}

	loader := NewDatasetLoader[string](dir, stringMapper)
	filtered := loader.Filter(func(ms ManifestSample) bool {
		return ms.Label == "positive"
	})

	ctx := context.Background()
	samples, err := filtered.All(ctx)
	if err != nil {
		t.Fatalf("All() error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	for _, s := range samples {
		if s.Label != "positive" {
			t.Errorf("expected label %q, got %q", "positive", s.Label)
		}
	}
}

func TestDatasetLoaderCustomManifestFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	m := DatasetManifest{
		Name:    "custom",
		Version: "1.0",
		Samples: []ManifestSample{{ID: "s1", Label: "x"}},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewDatasetLoader[string](dir, stringMapper, WithManifestFile("custom.json"))
	manifest, err := loader.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error: %v", err)
	}
	if manifest.Name != "custom" {
		t.Errorf("Name = %q, want %q", manifest.Name, "custom")
	}
}

func TestDatasetLoaderMissingManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loader := NewDatasetLoader[string](dir, stringMapper)

	ctx := context.Background()
	_, err := loader.All(ctx)
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

func TestDatasetLoaderBadJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewDatasetLoader[string](dir, stringMapper)
	ctx := context.Background()
	_, err := loader.All(ctx)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
}

func TestDatasetLoaderMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "bad-ds",
		Version: "1.0",
		Samples: []ManifestSample{
			{ID: "s1", File: "nonexistent.txt", Label: "positive"},
		},
	})

	loader := NewDatasetLoader[string](dir, stringMapper)
	ctx := context.Background()
	_, err := loader.All(ctx)
	if err == nil {
		t.Fatal("expected error for missing sample file, got nil")
	}
}

func TestDatasetLoaderSampleWithMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeManifest(t, dir, DatasetManifest{
		Name:    "meta-ds",
		Version: "1.0",
		Samples: []ManifestSample{
			{
				ID:     "s1",
				File:   "s1.txt",
				Label:  "pos",
				Source: "train",
				Meta:   map[string]any{"lang": "en"},
			},
		},
	})
	writeSampleFile(t, dir, "s1.txt", "content")

	loader := NewDatasetLoader[string](dir, stringMapper)
	ctx := context.Background()
	samples, err := loader.All(ctx)
	if err != nil {
		t.Fatalf("All() error: %v", err)
	}
	if samples[0].Source != "train" {
		t.Errorf("Source = %q, want %q", samples[0].Source, "train")
	}
	if samples[0].Metadata["lang"] != "en" {
		t.Errorf("Metadata[lang] = %v, want %q", samples[0].Metadata["lang"], "en")
	}
}
