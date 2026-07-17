package collect

import (
	"context"
	"os"
	"sync"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/stage"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Collector orchestrates item collection generically over the item type T: it
// streams each [stage.Source] through the configured transforms and pluggable
// [stage.Validator], classifies items real/AI, records stats in a
// [manifest.Manifest] cache, and publishes to the configured targets. Sources
// are streamed by a bounded worker pool; a single main loop owns the manifest,
// result, and progress, so no target or counter is shared across goroutines.
type Collector[T any] struct {
	sources    []stage.Source[T]
	transforms []stage.Transform[T, T]
	targets    []stage.Target[T]
	validator  stage.Validator[T]
	config     Config
	progress   Progress
	clock      util.Clock
}

// Option configures a [Collector].
type Option[T any] func(*Collector[T])

// WithSources adds sources to collect from.
func WithSources[T any](sources ...stage.Source[T]) Option[T] {
	return func(c *Collector[T]) { c.sources = append(c.sources, sources...) }
}

// WithTransforms adds transforms applied in order to every item.
func WithTransforms[T any](transforms ...stage.Transform[T, T]) Option[T] {
	return func(c *Collector[T]) { c.transforms = append(c.transforms, transforms...) }
}

// WithTargets adds publish targets.
func WithTargets[T any](targets ...stage.Target[T]) Option[T] {
	return func(c *Collector[T]) { c.targets = append(c.targets, targets...) }
}

// WithValidator sets the per-item validator every item must satisfy
// (fail-closed). A nil validator accepts every item.
func WithValidator[T any](v stage.Validator[T]) Option[T] {
	return func(c *Collector[T]) { c.validator = v }
}

// WithConfig sets the collector configuration.
func WithConfig[T any](config Config) Option[T] {
	return func(c *Collector[T]) { c.config = config }
}

// WithProgress sets the progress observer.
func WithProgress[T any](progress Progress) Option[T] {
	return func(c *Collector[T]) { c.progress = progress }
}

// WithClock injects the clock used for run timing (deterministic in tests).
func WithClock[T any](clock util.Clock) Option[T] {
	return func(c *Collector[T]) { c.clock = clock }
}

// New builds a collector from the given options.
func New[T any](opts ...Option[T]) *Collector[T] {
	c := &Collector[T]{
		config:   DefaultConfig(),
		progress: NullProgress{},
		clock:    util.SystemClock{},
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.config.OutputDir == "" {
		c.config.OutputDir = DefaultOutputDir
	}
	if c.config.Concurrency <= 0 {
		c.config.Concurrency = DefaultConcurrency
	}
	c.config.Limits = c.config.Limits.WithDefaults()
	return c
}

// Run collects every source through the worker pool, publishing each source's
// items to the targets as they complete, and persists the manifest cache. It
// fails closed: a source, validation, or target error aborts the run (the
// remaining sources are canceled) and no failed source's items are published.
// A resumable source that made partial progress is recorded as partial so a
// later run continues it.
func (c *Collector[T]) Run(ctx context.Context) (Result, error) {
	start := c.clock.Now()
	result := Result{
		SourceStats: map[string]manifest.SourceStats{},
		OutputDir:   c.config.OutputDir,
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := os.MkdirAll(c.config.OutputDir, 0o750); err != nil {
		return result, apperrors.Internal(err)
	}
	man, err := manifest.Load(c.config.OutputDir)
	if err != nil {
		return result, err
	}

	work := c.plan(man, &result)

	runErr := c.dispatch(ctx, man, &result, work)

	if err := man.Save(c.config.OutputDir); err != nil && runErr == nil {
		runErr = err
	}

	result.Duration = c.clock.Now().Sub(start)
	return result, runErr
}

// plan resolves each source's cache state, folding cache hits straight into the
// result and returning the work items the pool must stream (fresh or resumed).
func (c *Collector[T]) plan(man *manifest.Manifest, result *Result) []workItem[T] {
	work := make([]workItem[T], 0, len(c.sources))
	for i, src := range c.sources {
		key := stage.CacheKey(src)
		ceiling, hasMax := stage.MaxItems(src)

		if !c.config.Force {
			status := man.CacheStatusFor(src.Name(), key, ceiling, hasMax)
			switch status.Kind {
			case manifest.CacheDone:
				c.progress.OnSourceCached(i, src.Name(), status.Stats)
				result.CachedSources = append(result.CachedSources, src.Name())
				c.recordStats(result, src.Name(), status.Stats)
				continue
			case manifest.CachePartial:
				stage.Resume(src, status.Stats.FetchedOffset, status.Stats.Total)
				work = append(work, workItem[T]{index: i, src: src, cacheKey: key, resume: status.Stats})
				c.progress.OnSourceStart(i, src.Name(), ceiling, hasMax)
				continue
			case manifest.CacheNotCached:
			}
		}
		work = append(work, workItem[T]{index: i, src: src, cacheKey: key})
		c.progress.OnSourceStart(i, src.Name(), ceiling, hasMax)
	}
	return work
}

// dispatch runs the worker pool over work, folding each source event into the
// manifest, result, and targets from this single goroutine. It cancels the
// remaining work on the first error but always drains events and joins every
// worker before returning.
func (c *Collector[T]) dispatch(ctx context.Context, man *manifest.Manifest, result *Result, work []workItem[T]) error {
	if len(work) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	buffer := c.config.Limits.StreamBuffer
	workCh := make(chan workItem[T], buffer)
	eventCh := make(chan sourceEvent[T], buffer)

	var wg sync.WaitGroup
	for range min(c.config.Concurrency, len(work)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.worker(runCtx, workCh, eventCh)
		}()
	}

	go func() {
		defer close(workCh)
		for _, item := range work {
			select {
			case workCh <- item:
			case <-runCtx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(eventCh)
	}()

	var runErr error
	for ev := range eventCh {
		if err := c.handleEvent(ctx, man, result, ev); err != nil && runErr == nil {
			runErr = err
			cancel()
		}
	}
	return runErr
}

// handleEvent folds one worker result into the manifest, result, and targets.
// A failed source aborts the run; a resumable failed source with progress is
// recorded partial. A successful source is published and marked done.
func (c *Collector[T]) handleEvent(ctx context.Context, man *manifest.Manifest, result *Result, ev sourceEvent[T]) error {
	if ev.outcome == outcomeFailed {
		c.progress.OnSourceError(ev.index, ev.name, ev.err)
		if ev.resumable && ev.stats.Total > 0 {
			man.MarkPartial(ev.name, ev.cacheKey, ev.stats)
		}
		return ev.err
	}

	if err := c.publish(ctx, result, ev.items); err != nil {
		c.progress.OnSourceError(ev.index, ev.name, err)
		return err
	}
	man.MarkDone(ev.name, ev.cacheKey, ev.stats)
	c.recordStats(result, ev.name, ev.stats)
	c.progress.OnSourceProgress(ev.index, ev.stats.Total)
	c.progress.OnSourceDone(ev.index, ev.name, ev.stats)
	return nil
}

// publish streams one source's items to every target in order, recording each
// target's result. Targets are touched only from the main loop, so no target or
// result is accessed concurrently.
func (c *Collector[T]) publish(ctx context.Context, result *Result, items []T) error {
	if len(items) == 0 {
		return nil
	}
	for _, target := range c.targets {
		pub, err := target.Publish(ctx, streamOf(items))
		if err != nil {
			return err
		}
		result.PublishResults = append(result.PublishResults, pub)
		c.progress.OnPublishDone(target.Name(), pub)
	}
	return nil
}

// recordStats accumulates one source's stats into the run result.
func (c *Collector[T]) recordStats(result *Result, name string, stats manifest.SourceStats) {
	result.SourceStats[name] = stats
	result.TotalItems += stats.Total
	result.RealItems += stats.Real
	result.AIItems += stats.AI
}
