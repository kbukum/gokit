package collect

import (
	"time"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
)

// DefaultOutputDir is the default collector output directory.
const DefaultOutputDir = "dataset_build"

// DefaultSourceTimeout bounds a single source's collection when no override is given.
const DefaultSourceTimeout = 10 * time.Minute

// DefaultConcurrency is the number of sources collected in parallel when no override is given.
const DefaultConcurrency = 4

// Config configures a [Collector] run.
type Config struct {
	// OutputDir is where the manifest is written.
	OutputDir string
	// SourceTimeout bounds each source's streaming; zero disables the timeout.
	SourceTimeout time.Duration
	// Concurrency is the number of sources streamed in parallel; zero uses [DefaultConcurrency].
	Concurrency int
	// Force reprocesses sources even when the manifest reports them cached.
	Force bool
	// Limits bounds record and payload sizes and, via StreamBuffer, the work
	// and event channels that apply backpressure.
	Limits payload.Limits
}

// DefaultConfig returns the default collector configuration.
func DefaultConfig() Config {
	return Config{
		OutputDir:     DefaultOutputDir,
		SourceTimeout: DefaultSourceTimeout,
		Concurrency:   DefaultConcurrency,
		Limits:        payload.DefaultLimits(),
	}
}

// Result summarizes a completed [Collector] run.
type Result struct {
	// TotalItems is the number of items across all sources.
	TotalItems int
	// RealItems is the number of real (non-synthetic) items across all sources.
	RealItems int
	// AIItems is the number of AI (synthetic/augmented) items across all sources.
	AIItems int
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
