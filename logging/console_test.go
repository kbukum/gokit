package logging

import (
	"bytes"
	"strings"
	"testing"
)

func newConsoleLogger(buf *bytes.Buffer, service string, noColor bool) *Logger {
	cfg := &Config{Level: "trace", Format: "console", Output: "stdout", NoColor: noColor, Timestamp: false}
	l, err := New(cfg, service, WithWriter(buf))
	if err != nil {
		panic(err)
	}
	return l
}

func TestConsoleRendersLevelServiceAndAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newConsoleLogger(&buf, "eval", true)
	l.Info("started", map[string]any{"port": 8080})

	out := buf.String()
	if !strings.Contains(out, "[EVA]") {
		t.Errorf("expected 3-letter service tag, got %q", out)
	}
	if !strings.Contains(out, "[INF]") {
		t.Errorf("expected level tag, got %q", out)
	}
	if !strings.Contains(out, "started") || !strings.Contains(out, "port=8080") {
		t.Errorf("expected message and attr, got %q", out)
	}
}

func TestConsoleColorizesByDefault(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newConsoleLogger(&buf, "eval", false)
	l.Error("bad")

	if !strings.Contains(buf.String(), "\033[") {
		t.Errorf("expected ANSI color codes, got %q", buf.String())
	}
}

func TestConsoleLevelTags(t *testing.T) {
	t.Parallel()

	tags := map[string]func(*Logger){
		"[TRC]": func(l *Logger) { l.Trace("x") },
		"[DBG]": func(l *Logger) { l.Debug("x") },
		"[WRN]": func(l *Logger) { l.Warn("x") },
		"[ERR]": func(l *Logger) { l.Error("x") },
	}
	for tag, emit := range tags {
		var buf bytes.Buffer
		emit(newConsoleLogger(&buf, "svc", true))
		if !strings.Contains(buf.String(), tag) {
			t.Errorf("expected %s in %q", tag, buf.String())
		}
	}
}

func TestConsoleNeutralizesLogForging(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newConsoleLogger(&buf, "svc", true)
	l.Info("login\n[INF] forged", map[string]any{"user": "a\r\nb", "ansi": "x\x1b[31mred"})

	out := buf.String()
	if lines := strings.Count(strings.TrimRight(out, "\n"), "\n"); lines != 0 {
		t.Errorf("forged newlines produced extra log lines: %q", out)
	}
	if strings.ContainsAny(out, "\r\x1b") {
		t.Errorf("expected control characters neutralized, got %q", out)
	}
	if !strings.Contains(out, `login\n[INF] forged`) || !strings.Contains(out, `user=a\r\nb`) {
		t.Errorf("expected escaped forms, got %q", out)
	}
	if !strings.Contains(out, `ansi=x\x1b[31mred`) {
		t.Errorf("expected escaped ANSI escape, got %q", out)
	}
}
