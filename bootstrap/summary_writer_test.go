package bootstrap_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bootstrap"
)

func TestSummary_WithWriter_CapturesOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	s := bootstrap.NewSummaryWithOptions("test-svc", "0.1.0", bootstrap.WithWriter(&buf))
	s.SetStartupDuration(150 * time.Millisecond)

	// Display with no registry / container — just exercises the header path.
	s.DisplaySummary(nil, nil, nil)

	out := buf.String()
	if !strings.Contains(out, "test-svc") {
		t.Fatalf("expected service name in output, got: %q", out)
	}
	if !strings.Contains(out, "v0.1.0") {
		t.Fatalf("expected version in output, got: %q", out)
	}
}

func TestSummary_SetWriter_OverridesDefault(t *testing.T) {
	t.Parallel()

	s := bootstrap.NewSummary("svc", "1.0.0")
	var buf bytes.Buffer
	s.SetWriter(&buf)
	s.SetStartupDuration(time.Millisecond)
	s.DisplaySummary(nil, nil, nil)

	if buf.Len() == 0 {
		t.Fatal("expected output written to injected writer")
	}
}

func TestSummary_NilWriter_IgnoredBySetWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	s := bootstrap.NewSummaryWithOptions("svc", "1.0.0", bootstrap.WithWriter(&buf))
	s.SetWriter(nil) // nil should be ignored, not replace the existing writer.
	s.SetStartupDuration(time.Millisecond)
	s.DisplaySummary(nil, nil, nil)

	if buf.Len() == 0 {
		t.Fatal("nil SetWriter should not replace existing writer")
	}
}
