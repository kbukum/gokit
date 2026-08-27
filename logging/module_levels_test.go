package logging

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestModuleLevelManager(t *testing.T) {
	t.Parallel()

	m := NewModuleLevelManager(map[string]string{"DB": "debug", "bad": "nope"})

	if lvl, ok := m.Level("db"); !ok || lvl != slog.LevelDebug {
		t.Errorf("Level(db) = (%v,%v), want (debug,true)", lvl, ok)
	}
	if _, ok := m.Level("bad"); ok {
		t.Error("unrecognized level string should be ignored")
	}
	if _, ok := m.Level("missing"); ok {
		t.Error("unset module should report ok=false")
	}

	m.SetLevel("cache", "warn")
	if lvl, ok := m.Level("cache"); !ok || lvl != slog.LevelWarn {
		t.Errorf("Level(cache) = (%v,%v), want (warn,true)", lvl, ok)
	}
	m.SetLevel("cache", "nope") // ignored
	if lvl, _ := m.Level("cache"); lvl != slog.LevelWarn {
		t.Errorf("invalid SetLevel should be ignored, got %v", lvl)
	}
}

func TestModuleLevelOverridesGateOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{
		Level:        "info",
		Format:       "json",
		Output:       "stdout",
		Timestamp:    true,
		ModuleLevels: map[string]string{"db": "debug", "noisy": "warn"},
	}
	l, err := New(cfg, "svc", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.WithComponent("db").Debug("db-debug")     // more verbose override: kept
	l.Debug("plain-debug")                      // base info: dropped
	l.WithComponent("noisy").Info("noisy-info") // less verbose override: dropped
	l.WithComponent("noisy").Warn("noisy-warn") // meets override: kept

	lines := decodeLines(t, &buf)
	got := map[string]bool{}
	for _, m := range lines {
		got[m[FieldMessage].(string)] = true
	}
	if !got["db-debug"] {
		t.Error("more-verbose module override should keep db-debug")
	}
	if got["plain-debug"] {
		t.Error("base level should drop plain-debug")
	}
	if got["noisy-info"] {
		t.Error("less-verbose module override should drop noisy-info")
	}
	if !got["noisy-warn"] {
		t.Error("noisy-warn meets the override and should be kept")
	}
}
