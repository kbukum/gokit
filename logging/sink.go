package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// outputSink resolves the configured output to a writer and optional closer.
func outputSink(output Output) (io.Writer, io.Closer, error) {
	if err := output.Validate(); err != nil {
		return nil, nil, err
	}
	switch output.Type {
	case OutputTypeStderr:
		return os.Stderr, nil, nil
	case OutputTypeFile:
		file, err := fs.OpenAppend(output.Path)
		if err != nil {
			return nil, nil, apperrors.InvalidInput("logging.output.path", "failed to open log output file").WithCause(err)
		}
		if err := enforceOwnerOnly(file); err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		return file, file, nil
	default:
		return os.Stdout, nil, nil
	}
}

// enforceOwnerOnly tightens an opened log file to owner-only (0600) permissions.
// The 0600 creation mode only applies when the sink creates the file; an
// existing group- or world-readable file keeps its prior mode, so verify and
// restrict it after opening to keep potentially sensitive records private.
func enforceOwnerOnly(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return apperrors.InvalidInput("logging.output.path", "failed to stat log output file").WithCause(err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	if err := file.Chmod(0o600); err != nil {
		return apperrors.InvalidInput("logging.output.path", "failed to restrict log output file permissions").WithCause(err)
	}
	return nil
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
