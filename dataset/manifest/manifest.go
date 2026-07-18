package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// fileName is the on-disk manifest name inside an output directory.
const fileName = ".manifest.json"

// maxBytes bounds a manifest file read (1 MiB).
const maxBytes int64 = 1024 * 1024

// Status values recorded for a source entry.
const (
	statusDone    = "done"
	statusPartial = "partial"
)

// SourceStats records how many items a source produced and where a partial run left off so it can resume.
type SourceStats struct {
	// Total is the number of items collected.
	Total int `json:"total"`
	// Real is the number of non-synthetic items.
	Real int `json:"real"`
	// AI is the number of synthetic/augmented items.
	AI int `json:"ai"`
	// FetchedOffset is the resume offset for a partial run.
	FetchedOffset int `json:"fetched_offset"`
}

// SourceEntry is a manifest record for one source: its cache key, stats, and completion status.
type SourceEntry struct {
	// CacheKey fingerprints the source configuration this entry was built with.
	CacheKey string `json:"cache_key"`
	// Stats holds the collected counts.
	Stats SourceStats `json:"stats"`
	// Status is either done or partial.
	Status string `json:"status"`
}

// CacheKind classifies a source's cache state.
type CacheKind int

const (
	// CacheNotCached means the source has no usable cached entry.
	CacheNotCached CacheKind = iota
	// CachePartial means a resumable partial entry exists.
	CachePartial
	// CacheDone means a complete entry exists.
	CacheDone
)

// CacheStatus is the resolved cache state for a source, with the cached stats when present.
type CacheStatus struct {
	// Kind is the resolved cache classification.
	Kind CacheKind
	// Stats are the cached counts (zero when not cached).
	Stats SourceStats
}

// Manifest is the cache layer: it maps source names to their [SourceEntry].
type Manifest struct {
	// Sources maps a source name to its cache entry.
	Sources map[string]SourceEntry `json:"sources"`
}

// New returns an empty manifest.
func New() *Manifest {
	return &Manifest{Sources: map[string]SourceEntry{}}
}

// Load reads the manifest from dir, returning an empty manifest when none exists. The read is bounded to 1 MiB and malformed content fails closed.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, fileName)
	data, err := fs.ReadFileLimit(path, maxBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(), nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, apperrors.InvalidInput("manifest", "manifest file is not valid JSON").WithCause(err)
	}
	if m.Sources == nil {
		m.Sources = map[string]SourceEntry{}
	}
	return &m, nil
}

// Save writes the manifest atomically into dir.
func (m *Manifest) Save(dir string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return apperrors.Internal(err)
	}
	path := filepath.Join(dir, fileName)
	return fs.WriteAtomicReplace(path, data, "manifest-")
}

// MarkDone records a completed source entry.
func (m *Manifest) MarkDone(name, cacheKey string, stats SourceStats) {
	m.Sources[name] = SourceEntry{CacheKey: cacheKey, Stats: stats, Status: statusDone}
}

// MarkPartial records a resumable partial source entry.
func (m *Manifest) MarkPartial(name, cacheKey string, stats SourceStats) {
	m.Sources[name] = SourceEntry{CacheKey: cacheKey, Stats: stats, Status: statusPartial}
}

// CacheStatusFor resolves a source's cache state. A partial entry is promoted to done when it is within five items or 99% of a known ceiling.
func (m *Manifest) CacheStatusFor(name, cacheKey string, ceiling int, hasCeiling bool) CacheStatus {
	entry, ok := m.Sources[name]
	if !ok || entry.CacheKey != cacheKey {
		return CacheStatus{Kind: CacheNotCached}
	}
	switch entry.Status {
	case statusDone:
		return CacheStatus{Kind: CacheDone, Stats: entry.Stats}
	case statusPartial:
		if entry.Stats.Total <= 0 {
			return CacheStatus{Kind: CacheNotCached}
		}
		if hasCeiling && ceiling > 0 && nearComplete(entry.Stats.Total, ceiling) {
			return CacheStatus{Kind: CacheDone, Stats: entry.Stats}
		}
		return CacheStatus{Kind: CachePartial, Stats: entry.Stats}
	default:
		return CacheStatus{Kind: CacheNotCached}
	}
}

// nearComplete reports whether total is within five items or 99% of ceiling.
func nearComplete(total, ceiling int) bool {
	if total >= ceiling {
		return true
	}
	if ceiling-total <= 5 {
		return true
	}
	return float64(total) >= 0.99*float64(ceiling)
}
