// Package logging provides structured logging for gokit applications using zerolog.
//
// It supports multiple output formats (JSON, console), log level configuration,
// and component-scoped loggers with structured fields.
//
// # Configuration
//
//	logging:
//	  level: "info"
//	  format: "json"
//
// # Usage
//
//	log := logging.NewDefault("my-service")
//	log.Info("operation completed", logging.Fields("key", "value"))
//
//	// Component-scoped logger derived from a base logger.
//	dbLog := log.WithComponent("database")
//	dbLog.Debug("query executed")
//
// # Context and filtering
//
// [ContextWithTraceID] and its siblings carry trace, span, request, user, and correlation IDs
// on a context; [ComponentSpan] and [RequestSpan] derive loggers enriched from that context.
// [BuildDirectives] and [ParseDirectives] convert between a base level plus per-module overrides
// and the compact directive string used to configure level filtering from the environment.
package logging
