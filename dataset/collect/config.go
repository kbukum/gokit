package collect

import (
	"time"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
)

// DefaultOutputDir is the default collector output directory.
const DefaultOutputDir = "dataset_build"

// DefaultSourceTimeout bounds a single source's collection when no override is
// given.
const DefaultSourceTimeout = 10 * time.Minute

// Config configures a [Collector] run.
type Config struct {
	// OutputDir is where the manifest is written.
	OutputDir string
	// SourceTimeout bounds each source's streaming; zero disables the timeout.
	SourceTimeout time.Duration
	// Force reprocesses sources even when the manifest reports them cached.
	Force bool
	// Limits bounds record and payload sizes.
	Limits payload.Limits
}

// DefaultConfig returns the default collector configuration.
func DefaultConfig() Config {
	return Config{
		OutputDir:     DefaultOutputDir,
		SourceTimeout: DefaultSourceTimeout,
		Limits:        payload.DefaultLimits(),
	}
}

// Result summarizes a completed [Collector] run.
type Result struct {
	// TotalItems is the number of records across all sources.
	TotalItems int
	// SourceStats maps a source name to its collected stats.
	SourceStats map[string]manifest.SourceStats
	// CachedSources lists sources served from the cache.
	CachedSources []string
	// PublishResults holds each target's publish result.
	PublishResults []stage.PublishResult
	// Duration is the wall-clock run time.
	Duration time.Duration
	// OutputDir is the directory the manifest was written to.
	OutputDir string
}
