package collect

import (
	"context"
	"os"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/dataset/schema"
	"github.com/kbukum/gokit/dataset/stage"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/stream"
	"github.com/kbukum/gokit/util"
)

// Collector orchestrates record collection: it streams each
// [stage.Source] through the configured transforms and fail-closed schema
// validation, records stats in a [manifest.Manifest] cache, and publishes to
// the configured targets.
type Collector struct {
	sources    []stage.Source[record.Record]
	transforms []stage.Transform[record.Record, record.Record]
	targets    []stage.Target[record.Record]
	schema     *schema.Schema
	config     Config
	progress   Progress
	clock      util.Clock
}

// Option configures a [Collector].
type Option func(*Collector)

// WithSources adds sources to collect from.
func WithSources(sources ...stage.Source[record.Record]) Option {
	return func(c *Collector) { c.sources = append(c.sources, sources...) }
}

// WithTransforms adds record transforms applied in order.
func WithTransforms(transforms ...stage.Transform[record.Record, record.Record]) Option {
	return func(c *Collector) { c.transforms = append(c.transforms, transforms...) }
}

// WithTargets adds publish targets.
func WithTargets(targets ...stage.Target[record.Record]) Option {
	return func(c *Collector) { c.targets = append(c.targets, targets...) }
}

// WithSchema sets a schema that every record must satisfy (fail-closed).
func WithSchema(s *schema.Schema) Option {
	return func(c *Collector) { c.schema = s }
}

// WithConfig sets the collector configuration.
func WithConfig(config Config) Option {
	return func(c *Collector) { c.config = config }
}

// WithProgress sets the progress observer.
func WithProgress(progress Progress) Option {
	return func(c *Collector) { c.progress = progress }
}

// WithClock injects the clock used for run timing (deterministic in tests).
func WithClock(clock util.Clock) Option {
	return func(c *Collector) { c.clock = clock }
}

// New builds a collector from the given options.
func New(opts ...Option) *Collector {
	c := &Collector{
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
	c.config.Limits = c.config.Limits.WithDefaults()
	return c
}

// Run collects every source, publishes the collected records to each target,
// and only then persists the manifest cache. It fails closed: a source,
// validation, or target error aborts the run and leaves the cache unchanged.
func (c *Collector) Run(ctx context.Context) (Result, error) {
	start := c.clock.Now()
	result := Result{
		SourceStats: map[string]manifest.SourceStats{},
		OutputDir:   c.config.OutputDir,
	}

	if err := os.MkdirAll(c.config.OutputDir, 0o750); err != nil {
		return result, apperrors.Internal(err)
	}
	man, err := manifest.Load(c.config.OutputDir)
	if err != nil {
		return result, err
	}

	var collected []record.Record
	for i, src := range c.sources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		key := stage.CacheKey(src)
		ceiling, hasMax := stage.MaxItems(src)

		if !c.config.Force {
			status := man.CacheStatusFor(src.Name(), key, ceiling, hasMax)
			if status.Kind == manifest.CacheDone {
				c.progress.OnSourceCached(i, src.Name(), status.Stats)
				result.CachedSources = append(result.CachedSources, src.Name())
				result.SourceStats[src.Name()] = status.Stats
				result.TotalItems += status.Stats.Total
				continue
			}
		}

		records, err := c.processSource(ctx, i, src)
		if err != nil {
			c.progress.OnSourceError(i, src.Name(), err)
			return result, err
		}
		stats := manifest.SourceStats{Total: len(records), Real: len(records)}
		man.MarkDone(src.Name(), key, stats)
		c.progress.OnSourceDone(i, src.Name(), stats)
		result.SourceStats[src.Name()] = stats
		result.TotalItems += len(records)
		collected = append(collected, records...)
	}

	for _, target := range c.targets {
		pub, err := target.Publish(ctx, stream.FromSlice(collected))
		if err != nil {
			return result, err
		}
		result.PublishResults = append(result.PublishResults, pub)
		c.progress.OnPublishDone(target.Name(), pub)
	}

	if err := man.Save(c.config.OutputDir); err != nil {
		return result, err
	}

	result.Duration = c.clock.Now().Sub(start)
	return result, nil
}

// processSource streams one source through the transforms and schema
// validation, returning its collected records. The context bounds the source
// with SourceTimeout when configured.
func (c *Collector) processSource(ctx context.Context, index int, src stage.Source[record.Record]) ([]record.Record, error) {
	ceiling, hasMax := stage.MaxItems(src)
	c.progress.OnSourceStart(index, src.Name(), ceiling, hasMax)

	srcCtx := ctx
	if c.config.SourceTimeout > 0 {
		var cancel context.CancelFunc
		srcCtx, cancel = context.WithTimeout(ctx, c.config.SourceTimeout)
		defer cancel()
	}

	pipeline := src.Stream(srcCtx)
	for _, t := range c.transforms {
		pipeline = stage.ApplyTransform(pipeline, t)
	}

	var records []record.Record
	err := stream.ForEach(srcCtx, pipeline, func(_ context.Context, rec record.Record) error {
		if err := c.schema.Validate(rec); err != nil {
			return err
		}
		records = append(records, rec)
		c.progress.OnSourceProgress(index, len(records))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}
