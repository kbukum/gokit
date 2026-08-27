package logging

import (
	"log/slog"
	"strings"
)

// Custom slog levels extending the four stdlib levels
// ([slog.LevelDebug], [slog.LevelInfo], [slog.LevelWarn], [slog.LevelError])
// with the trace and fatal levels the unified gokit/rskit/pykit schema uses.
const (
	// LevelTrace is more verbose than [slog.LevelDebug].
	LevelTrace = slog.Level(-8)
	// LevelFatal is more severe than [slog.LevelError] and, at the facade, terminates the process.
	LevelFatal = slog.Level(12)
)

// levelNames maps the gokit levels to their canonical lowercase names.
var levelNames = map[slog.Level]string{
	LevelTrace:      "trace",
	slog.LevelDebug: "debug",
	slog.LevelInfo:  "info",
	slog.LevelWarn:  "warn",
	slog.LevelError: "error",
	LevelFatal:      "fatal",
}

// ParseLevel converts a level string to an [slog.Level].
// Unrecognized values fall back to [slog.LevelInfo] and report ok=false so
// callers can decide whether to treat the input as an error.
func ParseLevel(s string) (level slog.Level, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace, true
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	case "fatal":
		return LevelFatal, true
	default:
		return slog.LevelInfo, false
	}
}

// LevelName returns the canonical lowercase name for a level, mapping the
// custom trace/fatal levels and rounding any non-canonical level down to the
// nearest named severity (matching slog's own naming behavior).
func LevelName(level slog.Level) string {
	if name, ok := levelNames[level]; ok {
		return name
	}
	switch {
	case level < slog.LevelDebug:
		return "trace"
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	case level < LevelFatal:
		return "error"
	default:
		return "fatal"
	}
}
