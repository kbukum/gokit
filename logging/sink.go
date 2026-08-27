package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// outputWriter resolves the configured output name to a writer.
func outputWriter(output string) io.Writer {
	switch strings.ToLower(output) {
	case "stderr":
		return os.Stderr
	default:
		return os.Stdout
	}
}

// newSink builds the default sink handler for the configuration: a colored
// console handler for the "console"/"pretty" formats, otherwise the stdlib
// JSON or text handler. The returned handler honors the dynamic level via lv,
// writes to w, and normalizes attribute names to the unified schema.
func newSink(cfg *Config, serviceName string, lv slog.Leveler, w io.Writer) slog.Handler {
	format := strings.ToLower(cfg.Format)
	if format == "console" || format == FormatPretty {
		return newConsoleHandler(cfg, serviceName, lv, w)
	}

	opts := &slog.HandlerOptions{
		Level:       lv,
		AddSource:   cfg.Caller,
		ReplaceAttr: schemaReplaceAttr(cfg),
	}

	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	if serviceName != "" {
		h = h.WithAttrs([]slog.Attr{slog.String(FieldService, serviceName)})
	}
	return h
}

// schemaReplaceAttr normalizes the built-in slog keys to the unified schema
// (timestamp/level/message) and renders the custom trace/fatal level names. It
// also drops the timestamp attribute when timestamps are disabled.
func schemaReplaceAttr(cfg *Config) func(groups []string, a slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}
		switch a.Key {
		case slog.TimeKey:
			if !cfg.Timestamp {
				return slog.Attr{}
			}
			return slog.Attr{Key: FieldTimestamp, Value: a.Value}
		case slog.MessageKey:
			return slog.Attr{Key: FieldMessage, Value: a.Value}
		case slog.LevelKey:
			if level, ok := a.Value.Any().(slog.Level); ok {
				return slog.String(slog.LevelKey, strings.ToUpper(LevelName(level)))
			}
		}
		return a
	}
}
