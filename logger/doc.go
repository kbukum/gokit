// Package logger provides structured logging for gokit applications
// using zerolog.
//
// It supports multiple output formats (JSON, console), log level
// configuration, and component-scoped loggers with structured fields.
//
// # Configuration
//
//	logger:
//	  level: "info"
//	  format: "json"
//
// # Usage
//
//	log := logger.Get("my-component")
//	log.Info().Str("key", "value").Msg("operation completed")
package logger
