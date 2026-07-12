// Package logging provides structured logging for gokit applications
// using zerolog.
//
// It supports multiple output formats (JSON, console), log level
// configuration, and component-scoped loggers with structured fields.
//
// # Configuration
//
//	logging:
//	  level: "info"
//	  format: "json"
//
// # Usage
//
//	log := logging.Get("my-component")
//	log.Info().Str("key", "value").Msg("operation completed")
package logging
