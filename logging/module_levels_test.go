package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewModuleLevelManager_ParsesLevels(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{
		"database": "debug",
		"kafka":    "warn",
		"http":     "error",
	})

	tests := []struct {
		module string
		want   zerolog.Level
	}{
		{"database", zerolog.DebugLevel},
		{"kafka", zerolog.WarnLevel},
		{"http", zerolog.ErrorLevel},
	}
	for _, tc := range tests {
		lvl, ok := m.Level(tc.module)
		if !ok {
			t.Errorf("expected level override for %q", tc.module)
			continue
		}
		if lvl != tc.want {
			t.Errorf("Level(%q) = %v, want %v", tc.module, lvl, tc.want)
		}
	}
}

func TestModuleLevelManager_UnknownModuleReturnsFalse(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{
		"database": "debug",
	})

	_, ok := m.Level("unknown")
	if ok {
		t.Error("expected false for unknown module")
	}
}

func TestModuleLevelManager_CaseInsensitive(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{
		"Database": "debug",
	})

	lvl, ok := m.Level("database")
	if !ok {
		t.Fatal("expected level for 'database' (case-insensitive)")
	}
	if lvl != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", lvl)
	}

	lvl, ok = m.Level("DATABASE")
	if !ok {
		t.Fatal("expected level for 'DATABASE' (case-insensitive)")
	}
	if lvl != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", lvl)
	}
}

func TestModuleLevelManager_SetLevel(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{})

	m.SetLevel("cache", "warn")
	lvl, ok := m.Level("cache")
	if !ok {
		t.Fatal("expected level after SetLevel")
	}
	if lvl != zerolog.WarnLevel {
		t.Errorf("expected WarnLevel, got %v", lvl)
	}

	// Overwrite existing
	m.SetLevel("cache", "error")
	lvl, ok = m.Level("cache")
	if !ok {
		t.Fatal("expected level after SetLevel overwrite")
	}
	if lvl != zerolog.ErrorLevel {
		t.Errorf("expected ErrorLevel, got %v", lvl)
	}
}

func TestModuleLevelManager_SetLevel_InvalidIgnored(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{})
	m.SetLevel("cache", "not-a-level")
	_, ok := m.Level("cache")
	if ok {
		t.Error("expected no level for invalid level string")
	}
}

func TestModuleLevelManager_InvalidLevelIgnored(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{
		"database": "not-a-real-level",
	})

	_, ok := m.Level("database")
	if ok {
		t.Error("expected invalid level to be silently ignored")
	}
}

func TestModuleLevelManager_ConcurrentAccess(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{
		"database": "debug",
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.Level("database")
			m.Level("unknown")
		}()
		go func() {
			defer wg.Done()
			m.SetLevel("dynamic", "warn")
		}()
	}
	wg.Wait()
}

func TestWithComponent_ModuleLevelOverride(t *testing.T) {
	// Reset global level to allow debug messages through.
	zerolog.SetGlobalLevel(zerolog.TraceLevel)

	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)

	mlm := NewModuleLevelManager(map[string]string{
		"database": "debug",
	})

	l := &Logger{
		logger:       zl,
		service:      "test",
		moduleLevels: mlm,
	}

	// WithComponent("database") should lower level to debug
	dbLogger := l.WithComponent("database")
	dbLogger.Debug("debug msg")

	if buf.Len() == 0 {
		t.Error("expected debug message from database component to be logged")
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}
	if entry["component"] != "database" {
		t.Errorf("expected component=database, got %v", entry["component"])
	}
}

func TestWithComponent_NoModuleLevelOverride(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.InfoLevel)

	mlm := NewModuleLevelManager(map[string]string{
		"database": "debug",
	})

	l := &Logger{
		logger:       zl,
		service:      "test",
		moduleLevels: mlm,
	}

	// WithComponent("http") has no override; should keep InfoLevel
	httpLogger := l.WithComponent("http")
	httpLogger.Debug("should not appear")

	if buf.Len() != 0 {
		t.Error("expected debug message from http component to be suppressed")
	}
}

func TestWithComponent_ModuleLevelPropagation(t *testing.T) {
	mlm := NewModuleLevelManager(map[string]string{
		"database": "debug",
	})

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
		Masking: MaskingConfig{
			Enabled: false,
		},
	}
	l := New(cfg, "test")
	l.moduleLevels = mlm

	// WithComponent should propagate moduleLevels
	cl := l.WithComponent("handler")
	if cl.moduleLevels == nil {
		t.Error("expected moduleLevels to propagate through WithComponent")
	}

	// WithFields should propagate moduleLevels
	fl := l.WithFields(map[string]any{"key": "val"})
	if fl.moduleLevels == nil {
		t.Error("expected moduleLevels to propagate through WithFields")
	}

	// WithError should propagate moduleLevels
	el := l.WithError(nil)
	if el.moduleLevels == nil {
		t.Error("expected moduleLevels to propagate through WithError")
	}

	// WithMasker should propagate moduleLevels
	ml := l.WithMasker(nil)
	if ml.moduleLevels == nil {
		t.Error("expected moduleLevels to propagate through WithMasker")
	}
}

func TestNew_WithModuleLevels(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
		ModuleLevels: map[string]string{
			"database": "debug",
		},
		Masking: MaskingConfig{Enabled: false},
	}
	l := New(cfg, "test")
	if l.moduleLevels == nil {
		t.Error("expected moduleLevels to be set from config")
	}
}

func TestNew_WithoutModuleLevels(t *testing.T) {
	cfg := &Config{
		Level:   "info",
		Format:  "json",
		Output:  "stdout",
		Masking: MaskingConfig{Enabled: false},
	}
	l := New(cfg, "test")
	if l.moduleLevels != nil {
		t.Error("expected moduleLevels to be nil when not configured")
	}
}

func TestModuleLevelManager_EmptyMap(t *testing.T) {
	m := NewModuleLevelManager(map[string]string{})
	_, ok := m.Level("anything")
	if ok {
		t.Error("expected false for empty manager")
	}
}

func TestWithComponent_ModuleLevelRaisesLevel(t *testing.T) {
	// Test that a module level can also RAISE the level (e.g., suppress noisy module)
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel)

	mlm := NewModuleLevelManager(map[string]string{
		"noisy": "error",
	})

	l := &Logger{
		logger:       zl,
		service:      "test",
		moduleLevels: mlm,
	}

	noisyLogger := l.WithComponent("noisy")
	noisyLogger.Info("should not appear")

	if buf.Len() != 0 {
		lines := strings.TrimSpace(buf.String())
		t.Errorf("expected info from noisy module to be suppressed, got: %s", lines)
	}
}
