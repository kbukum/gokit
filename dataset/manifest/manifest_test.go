package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := New()
	m.MarkDone("src", "key1", SourceStats{Total: 5, Real: 3, AI: 2, FetchedOffset: 5})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	status := loaded.CacheStatusFor("src", "key1", 0, false)
	if status.Kind != CacheDone {
		t.Fatalf("CacheStatusFor = %v; want CacheDone", status.Kind)
	}
	if got, want := status.Stats, (SourceStats{Total: 5, Real: 3, AI: 2, FetchedOffset: 5}); got != want {
		t.Fatalf("stats = %+v; want %+v", got, want)
	}
}

func TestPartialOffsetRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := New()
	m.MarkPartial("src", "key1", SourceStats{Total: 40, Real: 40, FetchedOffset: 40})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	status := loaded.CacheStatusFor("src", "key1", 100, true)
	if status.Kind != CachePartial {
		t.Fatalf("CacheStatusFor = %v; want CachePartial", status.Kind)
	}
	if status.Stats.FetchedOffset != 40 {
		t.Fatalf("FetchedOffset = %d; want 40", status.Stats.FetchedOffset)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Parallel()
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(m.Sources) != 0 {
		t.Fatalf("expected empty manifest, got %d sources", len(m.Sources))
	}
}

func TestLoadMalformedFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for malformed manifest")
	}
}

func TestCacheStatusForDonePartialNotCached(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkDone("done", "k", SourceStats{Total: 10})
	m.MarkPartial("partial", "k", SourceStats{Total: 3})

	if s := m.CacheStatusFor("done", "k", 0, false); s.Kind != CacheDone {
		t.Fatalf("done -> %v; want CacheDone", s.Kind)
	}
	if s := m.CacheStatusFor("partial", "k", 100, true); s.Kind != CachePartial {
		t.Fatalf("partial -> %v; want CachePartial", s.Kind)
	}
	if s := m.CacheStatusFor("absent", "k", 0, false); s.Kind != CacheNotCached {
		t.Fatalf("absent -> %v; want CacheNotCached", s.Kind)
	}
}

func TestCacheStatusPartialPromotedNearComplete(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkPartial("p", "k", SourceStats{Total: 98})
	if s := m.CacheStatusFor("p", "k", 100, true); s.Kind != CacheDone {
		t.Fatalf("near-complete partial -> %v; want CacheDone", s.Kind)
	}

	m.MarkPartial("z", "k", SourceStats{Total: 0})
	if s := m.CacheStatusFor("z", "k", 100, true); s.Kind != CacheNotCached {
		t.Fatalf("zero-item partial -> %v; want CacheNotCached", s.Kind)
	}
}

func TestCacheStatusKeyMismatchNotCached(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkDone("s", "old", SourceStats{Total: 1})
	if s := m.CacheStatusFor("s", "new", 0, false); s.Kind != CacheNotCached {
		t.Fatalf("key mismatch -> %v; want CacheNotCached", s.Kind)
	}
	if s := m.CacheStatusFor("absent", "old", 0, false); s.Kind != CacheNotCached {
		t.Fatalf("absent source -> %v; want CacheNotCached", s.Kind)
	}
}
