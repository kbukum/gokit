package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestContextValueIsMasked guards the handler ordering: context enrichment runs
// before masking, so a sensitive value folded from the context (here an email
// carried as the user ID) is redacted before it reaches any sink.
func TestContextValueIsMasked(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{
		Level:   "info",
		Format:  "json",
		Output:  "stdout",
		Masking: MaskingConfig{Enabled: true, Replacement: "***REDACTED***"},
	}
	l, err := New(cfg, "test", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ContextWithUserID(context.Background(), "alice@example.com")
	l.InfoCtx(ctx, "login")

	if strings.Contains(buf.String(), "alice@example.com") {
		t.Errorf("context-derived sensitive value leaked unmasked: %q", buf.String())
	}
	m := decodeLine(t, &buf)
	if m[FieldUserID] != "***@***.***" {
		t.Errorf("user_id should be masked, got %v", m[FieldUserID])
	}
}

// TestNewDefaultMasks verifies NewDefault applies the secure default so its
// handler chain includes the masker. It reconstructs NewDefault's exact config
// against a buffer to inspect the rendered output.
func TestNewDefaultMasks(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{Level: "info", Format: "json", Output: "stdout", Timestamp: true}
	cfg.ApplyDefaults() // the step NewDefault performs to enable the secure baseline
	l, err := New(cfg, "svc", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Info("op", map[string]any{"password": "hunter2"})
	if strings.Contains(buf.String(), "hunter2") {
		t.Errorf("NewDefault config should mask secrets, got %q", buf.String())
	}
	m := decodeLine(t, &buf)
	if m["password"] != "***REDACTED***" {
		t.Errorf("password should be masked, got %v", m["password"])
	}
}

// TestMaskLogValuerGroup ensures a slog.LogValuer that resolves to a group is
// masked recursively rather than being treated as an opaque scalar.
func TestMaskLogValuerGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{
		Level:   "info",
		Format:  "json",
		Output:  "stdout",
		Masking: MaskingConfig{Enabled: true, Replacement: "***REDACTED***"},
	}
	l, err := New(cfg, "test", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Slog().Info("creds", slog.Any("creds", groupValuer{}))

	if strings.Contains(buf.String(), "s3cr3t") {
		t.Errorf("nested secret in LogValuer group leaked unmasked: %q", buf.String())
	}
}

// groupValuer is a slog.LogValuer that resolves to a group containing a
// sensitive key.
type groupValuer struct{}

func (groupValuer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("password", "s3cr3t"))
}

// TestFanoutHonorsBranchEnabled verifies a debug-enabled consumer handler does
// not force debug records onto the info-level default sink.
func TestFanoutHonorsBranchEnabled(t *testing.T) {
	t.Parallel()

	var defaultSink bytes.Buffer
	consumer := &capturingHandler{level: slog.LevelDebug}
	cfg := &Config{Level: "info", Format: "json", Output: "stdout", Timestamp: true}
	l, err := New(cfg, "test", WithWriter(&defaultSink), WithHandler(consumer))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Debug("debug line")

	if strings.Contains(defaultSink.String(), "debug line") {
		t.Errorf("info default sink should not emit debug record, got %q", defaultSink.String())
	}
	if consumer.count == 0 {
		t.Error("debug-enabled consumer handler should have received the debug record")
	}
}

// TestModuleOverrideStillReachesSinks verifies a per-module debug override
// still lets debug records through to the default sink after the fanout began
// honoring per-branch Enabled.
func TestModuleOverrideStillReachesSinks(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{
		Level:        "info",
		Format:       "json",
		Output:       "stdout",
		Timestamp:    true,
		ModuleLevels: map[string]string{"db": "debug"},
	}
	l, err := New(cfg, "test", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.WithComponent("db").Debug("query")
	if !strings.Contains(buf.String(), "query") {
		t.Errorf("module override should let debug through to the sink, got %q", buf.String())
	}

	buf.Reset()
	l.WithComponent("other").Debug("noise")
	if strings.Contains(buf.String(), "noise") {
		t.Errorf("non-overridden component should stay at base level, got %q", buf.String())
	}
}

// TestModuleOverrideGatesCustomHandler verifies a per-module level override
// applies to consumer-supplied handlers too: a component pinned to warn must
// not emit a debug record to a debug-enabled custom sink, matching the
// documented "still governed by module-level policy" guarantee.
func TestModuleOverrideGatesCustomHandler(t *testing.T) {
	t.Parallel()

	var defaultSink bytes.Buffer
	consumer := &capturingHandler{level: slog.LevelDebug}
	cfg := &Config{
		Level:        "debug",
		Format:       "json",
		Output:       "stdout",
		Timestamp:    true,
		ModuleLevels: map[string]string{"quiet": "warn"},
	}
	l, err := New(cfg, "test", WithWriter(&defaultSink), WithHandler(consumer))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.WithComponent("quiet").Debug("suppressed")
	if consumer.count != 0 {
		t.Errorf("warn-pinned component should not reach the debug custom handler, got %d records", consumer.count)
	}

	l.WithComponent("loud").Debug("delivered")
	if consumer.count == 0 {
		t.Error("non-overridden component at debug should reach the custom handler")
	}
}

// capturingHandler is a minimal slog.Handler that counts records at or above
// its level, used to assert per-branch Enabled semantics.
type capturingHandler struct {
	level slog.Level
	count int
}

func (h *capturingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *capturingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.count++
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

// TestConsoleGroupedBoundAttrKey guards that a bound attribute qualified at bind
// time is not re-prefixed with the group at emit time (db.host, not db.db.host).
func TestConsoleGroupedBoundAttrKey(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{Level: "info", Format: "console", Output: "stdout", NoColor: true}
	l, err := New(cfg, "test", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Slog().WithGroup("db").With("host", "localhost").Info("connected")

	out := buf.String()
	if !strings.Contains(out, "db.host=localhost") {
		t.Errorf("expected db.host=localhost, got %q", out)
	}
	if strings.Contains(out, "db.db.host") {
		t.Errorf("bound attr double-prefixed with group: %q", out)
	}
}

// TestConsoleAttrBeforeGroupNotMoved guards that an attribute bound before a
// group is opened keeps its unqualified key.
func TestConsoleAttrBeforeGroupNotMoved(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{Level: "info", Format: "console", Output: "stdout", NoColor: true}
	l, err := New(cfg, "test", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Slog().With("a", 1).WithGroup("g").Info("m")

	out := buf.String()
	if !strings.Contains(out, "a=1") {
		t.Errorf("expected a=1 unqualified, got %q", out)
	}
	if strings.Contains(out, "g.a=1") {
		t.Errorf("attr bound before group was pulled into it: %q", out)
	}
}

// TestSamplingPerLevelBudget verifies each severity level has an independent
// sampling budget, so a burst of info records cannot suppress a later error.
func TestSamplingPerLevelBudget(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := &Config{
		Level:     "debug",
		Format:    "json",
		Output:    "stdout",
		Timestamp: true,
		Sampling:  SamplingConfig{Enabled: true, InitialRate: 2, ThereafterRate: 0},
	}
	l, err := New(cfg, "test", WithWriter(&buf), WithClock(func() time.Time { return fixed }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Exhaust the info budget within the window.
	for i := 0; i < 5; i++ {
		l.Info("info burst")
	}
	// An error in the same window must still be admitted (its own budget).
	l.Error("error still logged")

	lines := decodeLines(t, &buf)
	var infos, errorsCount int
	for _, m := range lines {
		switch strings.ToLower(m[FieldLevel].(string)) {
		case "info":
			infos++
		case "error":
			errorsCount++
		}
	}
	if infos != 2 {
		t.Errorf("expected 2 info records admitted (burst=2), got %d", infos)
	}
	if errorsCount != 1 {
		t.Errorf("error record should not be starved by info burst, got %d", errorsCount)
	}
}
