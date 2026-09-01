package logging

import (
	"bytes"
	"testing"
	"time"
)

func TestSamplingBurstThenThereafter(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	clock := func() time.Time { return now }

	var buf bytes.Buffer
	cfg := &Config{
		Level:    "info",
		Format:   "json",
		Output:   OutputStdout(),
		Sampling: SamplingConfig{Enabled: true, InitialRate: 2, ThereafterRate: 3},
	}
	l, err := New(cfg, "svc", WithWriter(&buf), WithClock(clock))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Same instant: admit first 2 (burst), then every 3rd → counts 1,2,5,8.
	for i := 0; i < 10; i++ {
		l.Info("tick")
	}
	if got := len(decodeLines(t, &buf)); got != 4 {
		t.Fatalf("within one period admitted %d, want 4", got)
	}

	// New period resets the counter.
	buf.Reset()
	now = now.Add(time.Second)
	l.Info("next-period")
	if got := len(decodeLines(t, &buf)); got != 1 {
		t.Fatalf("new period admitted %d, want 1", got)
	}
}

func TestSamplingDisabledPassesEverything(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	for i := 0; i < 50; i++ {
		l.Info("all")
	}
	if got := len(decodeLines(t, &buf)); got != 50 {
		t.Errorf("admitted %d, want 50 when sampling disabled", got)
	}
}

func TestSamplingBudgetSharedAcrossDerivedLoggers(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	clock := func() time.Time { return now }

	var buf bytes.Buffer
	cfg := &Config{
		Level:    "info",
		Format:   "json",
		Output:   OutputStdout(),
		Sampling: SamplingConfig{Enabled: true, InitialRate: 2, ThereafterRate: 0},
	}
	root, err := New(cfg, "svc", WithWriter(&buf), WithClock(clock))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Derived loggers must draw from the same burst budget as the root, not get
	// a fresh allowance each — otherwise the global rate limit would not hold.
	a := root.WithComponent("a")
	b := root.WithFields(map[string]any{"k": "v"})

	root.Info("r")
	a.Info("a")
	b.Info("b")
	root.Info("r2")

	// Burst of 2 within the single period, ThereafterRate 0 drops the rest, so
	// exactly 2 records survive across all three loggers combined.
	if got := len(decodeLines(t, &buf)); got != 2 {
		t.Fatalf("shared budget admitted %d across derived loggers, want 2", got)
	}
}
