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
package logging
