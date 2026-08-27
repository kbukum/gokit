package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCtxLevelMethods(t *testing.T) {
	t.Parallel()

	ctx := ContextWithRequestID(context.Background(), "req-7")
	emitters := map[string]func(*Logger){
		"DBG": func(l *Logger) { l.DebugCtx(ctx, "m") },
		"INF": func(l *Logger) { l.InfoCtx(ctx, "m") },
		"WRN": func(l *Logger) { l.WarnCtx(ctx, "m") },
		"ERR": func(l *Logger) { l.ErrorCtx(ctx, "m") },
		"TRC": func(l *Logger) { l.TraceCtx(ctx, "m") },
	}
	for wantLevel, emit := range emitters {
		var buf bytes.Buffer
		emit(newBufferedLogger(&buf, "trace", "json"))
		m := decodeLine(t, &buf)
		if m[FieldLevel] != wantLevel && !strings.EqualFold(m[FieldLevel].(string), levelWord(wantLevel)) {
			t.Errorf("level = %v, want %s", m[FieldLevel], wantLevel)
		}
		if m[FieldRequestID] != "req-7" {
			t.Errorf("ctx request_id not folded in: %v", m)
		}
	}
}

func levelWord(short string) string {
	switch short {
	case "DBG":
		return "debug"
	case "INF":
		return "info"
	case "WRN":
		return "warn"
	case "ERR":
		return "error"
	case "TRC":
		return "trace"
	}
	return short
}

// TestFatalDoesNotExit verifies that Fatal logs at the fatal level and returns
// to the caller rather than terminating the process — the library leaves the
// exit decision to the application entry point.
func TestFatalDoesNotExit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "trace", "json")
	l.Fatal("fatal boom", map[string]any{"reason": "shutdown"})

	m := decodeLine(t, &buf)
	if !strings.EqualFold(m[FieldLevel].(string), "fatal") {
		t.Errorf("level = %v, want fatal", m[FieldLevel])
	}
	if m[FieldMessage] != "fatal boom" {
		t.Errorf("message = %v, want %q", m[FieldMessage], "fatal boom")
	}
	if m["reason"] != "shutdown" {
		t.Errorf("reason field missing: %v", m)
	}
}
