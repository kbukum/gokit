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
	m.MarkDone("src", "key1", SourceStats{Total: 5, Real: 5})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	stats, ok := loaded.IsCached("src", "key1")
	if !ok || stats.Total != 5 {
		t.Fatalf("IsCached = %+v, %v; want total 5", stats, ok)
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

func TestIsCachedRejectsKeyMismatch(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkDone("src", "key1", SourceStats{Total: 1})
	if _, ok := m.IsCached("src", "different"); ok {
		t.Fatal("cache key mismatch should not be cached")
	}
	if _, ok := m.IsCached("absent", "key1"); ok {
		t.Fatal("absent source should not be cached")
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
}
