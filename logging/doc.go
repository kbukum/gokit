// Package logging provides structured logging for gokit applications built on
// the standard library's [log/slog].
//
// The injected [Logger] is a thin ergonomic facade over *slog.Logger. Its
// value-add behavior — sensitive-data masking, rate sampling, per-module
// levels, context correlation, and OTLP export — is implemented as a composable
// [slog.Handler] chain, which is the idiomatic slog extension seam. slog is the
// default, and bring-your-own is first-class.
//
// # Pluggability
//
// The logging backend is a stable port, not a hardwired library:
//
//   - [WithHandler] adds a consumer-supplied [slog.Handler] as an extra sink,
//     still governed by gokit's masking/sampling/level policy.
//   - [WithBaseSink] replaces the default JSON/console output entirely.
//   - [Logger.Slog] exposes the underlying *slog.Logger for stdlib-native use.
//
// A consumer can therefore route logs to zerolog, zap, or a custom sink through
// an [slog.Handler] without editing the kit. There is no process-global logger
// on the consumer path — construct a [Logger] and inject it.
//
// # Usage
//
//	log, err := logging.New(cfg, "my-service")
//	if err != nil {
//		return err
//	}
//	defer log.Close()
//	log.Info("operation completed", logging.Fields("key", "value"))
//
//	dbLog := log.WithComponent("database")
//	dbLog.DebugCtx(ctx, "query executed")
//
// # Context and filtering
//
// [ContextWithTraceID] and its siblings carry trace, span, request, user, and
// correlation IDs on a context; the *Ctx logging methods fold those identifiers
// into records automatically, and [Logger.ComponentSpan] / [Logger.RequestSpan]
// derive enriched loggers. [BuildDirectives] and [ParseDirectives] convert
// between a base level plus per-module overrides and the compact directive
// string used to configure level filtering from the environment.
package logging
