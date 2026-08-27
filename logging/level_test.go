package logging

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want slog.Level
		ok   bool
	}{
		{"trace", LevelTrace, true},
		{"debug", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"fatal", LevelFatal, true},
		{"  INFO ", slog.LevelInfo, true},
		{"bogus", slog.LevelInfo, false},
	}
	for _, tc := range cases {
		got, ok := ParseLevel(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseLevel(%q) = (%v,%v), want (%v,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLevelName(t *testing.T) {
	t.Parallel()

	cases := map[slog.Level]string{
		LevelTrace:          "trace",
		slog.LevelDebug:     "debug",
		slog.LevelInfo:      "info",
		slog.LevelWarn:      "warn",
		slog.LevelError:     "error",
		LevelFatal:          "fatal",
		slog.Level(-6):      "trace", // between trace and debug
		slog.LevelInfo + 1:  "info",  // rounds down to nearest named
		slog.LevelError + 1: "error",
	}
	for level, want := range cases {
		if got := LevelName(level); got != want {
			t.Errorf("LevelName(%v) = %q, want %q", level, got, want)
		}
	}
}
