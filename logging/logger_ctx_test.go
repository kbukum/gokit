package logging

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
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

// TestFatalExits runs Fatal in a subprocess and asserts a non-zero exit,
// the standard pattern for exercising an os.Exit path.
func TestFatalExits(t *testing.T) {
	if os.Getenv("GOKIT_LOG_FATAL_CHILD") == "1" {
		NewDefault("svc").Fatal("fatal boom")
		return // unreachable; Fatal calls os.Exit(1)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalExits")
	cmd.Env = append(os.Environ(), "GOKIT_LOG_FATAL_CHILD=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit from Fatal")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %v", err)
	}
}
