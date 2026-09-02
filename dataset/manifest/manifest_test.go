package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// cfg is a small object-shaped lineage used across the manifest tests.
func cfg(id string) json.RawMessage { return json.RawMessage(`{"id":"` + id + `"}`) }

func TestManifestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := New()
	m.MarkDone("src", cfg("key1"), SourceStats{Total: 5, Real: 3, AI: 2, FetchedOffset: 5})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	status := loaded.CacheStatusFor("src", cfg("key1"), 0, false)
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
	m.MarkPartial("src", cfg("key1"), SourceStats{Total: 40, Real: 40, FetchedOffset: 40})
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	status := loaded.CacheStatusFor("src", cfg("key1"), 100, true)
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
	m.MarkDone("done", cfg("k"), SourceStats{Total: 10})
	m.MarkPartial("partial", cfg("k"), SourceStats{Total: 3})

	if s := m.CacheStatusFor("done", cfg("k"), 0, false); s.Kind != CacheDone {
		t.Fatalf("done -> %v; want CacheDone", s.Kind)
	}
	if s := m.CacheStatusFor("partial", cfg("k"), 100, true); s.Kind != CachePartial {
		t.Fatalf("partial -> %v; want CachePartial", s.Kind)
	}
	if s := m.CacheStatusFor("absent", cfg("k"), 0, false); s.Kind != CacheNotCached {
		t.Fatalf("absent -> %v; want CacheNotCached", s.Kind)
	}
}

func TestCacheStatusPartialPromotedNearComplete(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkPartial("p", cfg("k"), SourceStats{Total: 98})
	if s := m.CacheStatusFor("p", cfg("k"), 100, true); s.Kind != CacheDone {
		t.Fatalf("near-complete partial -> %v; want CacheDone", s.Kind)
	}

	m.MarkPartial("z", cfg("k"), SourceStats{Total: 0})
	if s := m.CacheStatusFor("z", cfg("k"), 100, true); s.Kind != CacheNotCached {
		t.Fatalf("zero-item partial -> %v; want CacheNotCached", s.Kind)
	}
}

func TestCacheStatusConfigMismatchNotCached(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkDone("s", cfg("old"), SourceStats{Total: 1})
	if s := m.CacheStatusFor("s", cfg("new"), 0, false); s.Kind != CacheNotCached {
		t.Fatalf("config mismatch -> %v; want CacheNotCached", s.Kind)
	}
	if s := m.CacheStatusFor("absent", cfg("old"), 0, false); s.Kind != CacheNotCached {
		t.Fatalf("absent source -> %v; want CacheNotCached", s.Kind)
	}
}

// TestCacheStatusConfigStructuralEquality proves the lineage is compared by structure, not bytes:
// the same object with reordered keys and different whitespace still hits the cache.
func TestCacheStatusConfigStructuralEquality(t *testing.T) {
	t.Parallel()
	m := New()
	m.MarkDone("s", json.RawMessage(`{"a":1,"b":2}`), SourceStats{Total: 1})
	if s := m.CacheStatusFor("s", json.RawMessage(`{"b":2, "a":1}`), 0, false); s.Kind != CacheDone {
		t.Fatalf("reordered-key config -> %v; want CacheDone", s.Kind)
	}
}

// TestCacheStatusFailsClosedOnBadConfig proves cache matching fails closed when either lineage
// is empty or malformed JSON, so a corrupt fingerprint never satisfies a hit.
func TestCacheStatusFailsClosedOnBadConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		stored, query json.RawMessage
	}{
		"empty stored":     {stored: json.RawMessage(``), query: cfg("k")},
		"empty query":      {stored: cfg("k"), query: json.RawMessage(``)},
		"malformed stored": {stored: json.RawMessage(`{bad`), query: cfg("k")},
		"malformed query":  {stored: cfg("k"), query: json.RawMessage(`{bad`)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := New()
			m.MarkDone("s", tc.stored, SourceStats{Total: 1})
			if s := m.CacheStatusFor("s", tc.query, 0, false); s.Kind != CacheNotCached {
				t.Fatalf("bad config (%s) -> %v; want CacheNotCached", name, s.Kind)
			}
		})
	}
}

// TestSourceEntryMarshalsConfigAsObject is the golden shape check: the lineage serializes as a JSON
// object under "config" (not a string), matching the rskit SourceEntry.config wire contract.
func TestSourceEntryMarshalsConfigAsObject(t *testing.T) {
	t.Parallel()
	entry := SourceEntry{
		Config: json.RawMessage(`{"format":"jsonl","path":"/data/in.jsonl"}`),
		Stats:  SourceStats{Total: 2, Real: 2},
		Status: statusDone,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	const want = `{"config":{"format":"jsonl","path":"/data/in.jsonl"},"stats":{"total":2,"real":2,"ai":0,"fetched_offset":0},"status":"done"}`
	if string(data) != want {
		t.Fatalf("SourceEntry JSON =\n  %s\nwant\n  %s", data, want)
	}

	// Round-trip: the object survives Marshal/Unmarshal and still matches a cache query.
	var back SourceEntry
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !configsEqual(back.Config, entry.Config) {
		t.Fatalf("round-tripped config %s not equal to original %s", back.Config, entry.Config)
	}
}
