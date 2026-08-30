package logging

import (
	"errors"
	"io"
	"log/slog"
	"time"
)

// pipeline is the assembled logging chain: the root slog.Handler, the dynamic
// level control, and the OTLP provider (nil when disabled) for lifecycle.
type pipeline struct {
	handler slog.Handler
	level   *slog.LevelVar
	otlp    *OTLPProvider
	closers []io.Closer
}

// buildPipeline composes the handler chain for cfg and opts:
//
//	sampling → context → masking → fanout{ moduleGate(sink), moduleGate(otlp), BYO handlers… }
//
// Context enrichment runs before masking so identifiers folded from the context
// (which may themselves carry sensitive values) are redacted like any other
// attribute. Sampling and masking wrap the fanout so their policy applies to
// every branch. Per-module level overrides gate gokit's own governed sinks (the
// default/base sink and the OTLP branch) from inside the fanout, so a
// consumer-supplied handler keeps its own Enabled contract while the fanout
// still honors each branch's level individually. Bring-your-own handlers are
// added as fanout branches (opts.handlers) or fully replace the default sink
// (opts.baseSink); OTLP is one more branch when enabled.
func buildPipeline(cfg *Config, serviceName string, opts options) (pipeline, error) {
	level, _ := ParseLevel(cfg.Level)
	lv := new(slog.LevelVar)
	lv.Set(level)

	writer := opts.writer
	var closers []io.Closer
	if writer == nil && opts.baseSink == nil {
		var closer io.Closer
		var err error
		writer, closer, err = outputSink(cfg.Output)
		if err != nil {
			return pipeline{}, err
		}
		if closer != nil {
			closers = append(closers, closer)
		}
	}

	var manager *ModuleLevelManager
	if len(cfg.ModuleLevels) > 0 {
		manager = NewModuleLevelManager(cfg.ModuleLevels)
	}

	var branches []slog.Handler
	if opts.baseSink != nil {
		branches = append(branches, newModuleLevelHandler(opts.baseSink, manager))
	} else {
		sink := newSink(cfg, serviceName, lv, writer)
		branches = append(branches, newModuleLevelHandler(sink, manager))
	}

	var provider *OTLPProvider
	if cfg.OTLP.Enabled {
		p, err := NewOTLPProvider(OTLPProviderConfig{
			Exporter:    cfg.OTLP,
			ServiceName: serviceName,
			Environment: cfg.Environment,
			Version:     cfg.Version,
		})
		if err != nil {
			return pipeline{}, errors.Join(err, closeAll(closers))
		}
		provider = p
		branches = append(branches, newModuleLevelHandler(newOTLPHandler(provider, lv), manager))
	}
	for _, extra := range opts.handlers {
		branches = append(branches, newModuleLevelHandler(extra, manager))
	}

	h := newFanout(branches...)
	if cfg.Masking.Enabled || opts.masker != nil {
		masker := opts.masker
		if masker == nil {
			masker = NewDefaultMasker(cfg.Masking)
		}
		h = newMaskingHandler(h, masker)
	}
	h = newContextHandler(h)
	if cfg.Sampling.Enabled {
		h = newSamplingHandler(h, cfg.Sampling, opts.now)
	}

	return pipeline{handler: h, level: lv, otlp: provider, closers: closers}, nil
}

func closeAll(closers []io.Closer) error {
	var errs []error
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// options holds the constructor configuration set through the Option functions.
type options struct {
	handlers []slog.Handler
	baseSink slog.Handler
	masker   Masker
	now      func() time.Time
	writer   io.Writer
}

// Option customizes logger construction.
type Option func(*options)

// WithHandler adds a consumer-supplied [slog.Handler] as an additional sink.
// gokit's masking, sampling, and module-level policy still wrap it, so a
// bring-your-own backend (an slog handler bridging zerolog/zap, or a custom
// sink) receives the same governed records as the default sink. May be passed
// multiple times to fan out to several handlers.
func WithHandler(h slog.Handler) Option {
	return func(o *options) {
		if h != nil {
			o.handlers = append(o.handlers, h)
		}
	}
}

// WithBaseSink replaces the default JSON/console sink entirely with the given
// handler, while keeping gokit's middleware (masking, sampling, module levels,
// context enrichment) in front of it. Use this to own the terminal output
// format; use [WithHandler] to add a sink alongside the default.
func WithBaseSink(h slog.Handler) Option {
	return func(o *options) { o.baseSink = h }
}

// WithMasker enables masking with the given masker, overriding any masker the
// config would build. Passing a masker turns masking on even when the config
// leaves it disabled.
func WithMasker(m Masker) Option {
	return func(o *options) { o.masker = m }
}

// WithClock injects the clock used by sampling, for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// WithWriter directs the default sink to w instead of the configured
// stdout/stderr output. Useful for writing to a file, a buffer, or a test sink.
// It has no effect when [WithBaseSink] supplies a fully custom sink.
func WithWriter(w io.Writer) Option {
	return func(o *options) { o.writer = w }
}
