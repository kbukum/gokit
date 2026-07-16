package render_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	yaml "go.yaml.in/yaml/v3"

	"github.com/kbukum/gokit/cli/render"
	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

func TestOutputTableRenders(t *testing.T) {
	t.Parallel()
	table := render.NewOutputTable("Name", "Count").
		AddRow("real", "500").
		AddRow("ai", "500")
	out := table.String()
	if !strings.Contains(out, "Name") || !strings.Contains(out, "500") {
		t.Errorf("table missing content:\n%s", out)
	}
}

func TestOutputTableASCIIBorders(t *testing.T) {
	t.Parallel()
	out := render.NewOutputTable("A", "B").
		WithGlyphs(theme.NewGlyphs(false)).
		AddRow("x", "y").
		String()
	if strings.ContainsAny(out, "─│┌┬┐├┼┤└┴┘") {
		t.Errorf("ASCII table must not contain box-drawing runes:\n%s", out)
	}
	if !strings.Contains(out, "+") || !strings.Contains(out, "|") {
		t.Errorf("ASCII table must use +/| borders:\n%s", out)
	}
}

func TestOutputTableNormalizesRowWidth(t *testing.T) {
	t.Parallel()
	table := render.NewOutputTable("A", "B").
		AddRow("only-one").
		AddRow("x", "y", "extra")
	out := table.String()
	var width int
	for i, line := range strings.Split(out, "\n") {
		w := utf8.RuneCountInString(line)
		if i == 0 {
			width = w
			continue
		}
		if w != width {
			t.Errorf("ragged line %d width=%d want=%d:\n%s", i, w, width, out)
		}
	}
	if strings.Contains(out, "extra") {
		t.Error("overflow cell must be truncated")
	}
}

func TestOutputTableTitle(t *testing.T) {
	t.Parallel()
	out := render.NewOutputTable("X").WithTitle("Report").AddRow("v").String()
	if !strings.HasPrefix(out, "\nReport\n") {
		t.Errorf("title must lead the table:\n%s", out)
	}
}

func TestOutputKVRenders(t *testing.T) {
	t.Parallel()
	out := render.NewOutputKV().
		Add("Output", "/tmp/dataset").
		Add("Preset", "image").
		String()
	if !strings.Contains(out, "Output") || !strings.Contains(out, "/tmp/dataset") {
		t.Errorf("kv missing content:\n%s", out)
	}
	// Keys are right-aligned, so the shorter key gets leading padding.
	if !strings.Contains(out, " Preset:  image") {
		t.Errorf("kv must right-align keys:\n%s", out)
	}
}

func TestExitCodeForErrorMapsCodes(t *testing.T) {
	t.Parallel()
	cases := map[error]render.ExitCode{
		errors.NotFound("repo", "missing"): render.ExitNotFound,
		errors.InvalidInput("f", "bad"):    render.ExitUsage,
		errors.Unauthorized(""):            render.ExitPermission,
		errors.Conflict("dup"):             render.ExitConflict,
		errors.RateLimited():               render.ExitRateLimited,
		errors.Timeout("op"):               render.ExitTimeout,
		errors.Canceled("op"):              render.ExitCanceled,
		errors.ServiceUnavailable("svc"):   render.ExitUnavailable,
		errors.Internal(nil):               render.ExitFailure,
	}
	for err, want := range cases {
		if got := render.ExitCodeForError(err); got != want {
			t.Errorf("ExitCodeForError(%v) = %d, want %d", err, got, want)
		}
	}
	if render.ExitCodeForError(nil) != render.ExitSuccess {
		t.Error("nil error must be ExitSuccess")
	}
	if render.ExitCodeForError(errTest("raw")) != render.ExitFailure {
		t.Error("non-AppError must map to ExitFailure")
	}
}

func TestErrorRendererSameExitAcrossFormats(t *testing.T) {
	t.Parallel()
	err := errors.NotFound("repo", "missing")
	for _, format := range []render.OutputFormat{render.FormatText, render.FormatJSON, render.FormatYAML} {
		rendered, code := render.NewErrorRenderer(format).Render(err)
		if code != render.ExitNotFound {
			t.Errorf("format %v exit = %d, want %d", format, code, render.ExitNotFound)
		}
		if !strings.Contains(rendered, "not found") && !strings.Contains(rendered, "NOT_FOUND") {
			t.Errorf("format %v rendered missing error: %q", format, rendered)
		}
	}
}

func TestErrorRendererTextFormat(t *testing.T) {
	t.Parallel()
	rendered, _ := render.NewErrorRenderer(render.FormatText).Render(errors.Internal(nil))
	if !strings.HasPrefix(rendered, "error[INTERNAL_ERROR]:") {
		t.Errorf("text render = %q", rendered)
	}
}

func TestErrorRendererJSONIsValidAndEscaped(t *testing.T) {
	t.Parallel()
	err := errors.Validation(`boom "quoted"`)
	rendered, exit := render.NewErrorRenderer(render.FormatJSON).Render(err)
	var payload map[string]any
	if e := json.Unmarshal([]byte(rendered), &payload); e != nil {
		t.Fatalf("invalid json %q: %v", rendered, e)
	}
	if payload["code"] != "INVALID_INPUT" {
		t.Errorf("code = %v", payload["code"])
	}
	if payload["message"] != `boom "quoted"` {
		t.Errorf("message = %v", payload["message"])
	}
	if int(payload["exit_code"].(float64)) != exit.Int() {
		t.Errorf("exit_code = %v, want %d", payload["exit_code"], exit.Int())
	}
}

func TestErrorRendererYAMLCarriesFields(t *testing.T) {
	t.Parallel()
	rendered, _ := render.NewErrorRenderer(render.FormatYAML).Render(errors.Internal(nil))
	var payload map[string]any
	if e := yaml.Unmarshal([]byte(rendered), &payload); e != nil {
		t.Fatalf("invalid yaml %q: %v", rendered, e)
	}
	if payload["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %v", payload["code"])
	}
	if payload["exit_code"] != 1 {
		t.Errorf("exit_code = %v", payload["exit_code"])
	}
}

func TestErrorRendererWrapsNonAppError(t *testing.T) {
	t.Parallel()
	rendered, code := render.NewErrorRenderer(render.FormatText).Render(errTest("raw"))
	if code != render.ExitFailure {
		t.Errorf("non-app error exit = %d", code)
	}
	if !strings.Contains(rendered, "INTERNAL_ERROR") {
		t.Errorf("non-app error render = %q", rendered)
	}
}

func TestOutputFormatRoundTrip(t *testing.T) {
	t.Parallel()
	for _, f := range []render.OutputFormat{render.FormatText, render.FormatJSON, render.FormatYAML} {
		got, ok := render.ParseOutputFormat(f.String())
		if !ok || got != f {
			t.Errorf("round trip %v: got %v ok=%v", f, got, ok)
		}
	}
	if _, ok := render.ParseOutputFormat("xml"); ok {
		t.Error("unknown format must not parse")
	}
}

func TestOutputFormatStringForUnknown(t *testing.T) {
	t.Parallel()
	if got := render.OutputFormat(99).String(); !strings.Contains(got, "99") {
		t.Errorf("unknown format string = %q", got)
	}
}

func TestErrorRendererFallsBackOnUnmarshalableDetails(t *testing.T) {
	t.Parallel()
	// A detail value whose marshalers fail forces the marshal-error fallback
	// path in both structured formats without panicking the encoder.
	err := errors.Internal(nil).WithDetail("bad", failMarshal{})
	for _, format := range []render.OutputFormat{render.FormatJSON, render.FormatYAML} {
		rendered, code := render.NewErrorRenderer(format).Render(err)
		if code != render.ExitFailure {
			t.Errorf("format %v exit = %d, want %d", format, code, render.ExitFailure)
		}
		if !strings.Contains(rendered, "INTERNAL_ERROR") {
			t.Errorf("format %v fallback missing code: %q", format, rendered)
		}
	}
}

func TestOutputKVClampsWiderValueKey(t *testing.T) {
	t.Parallel()
	// A single key needs no padding; assert the pad-clamp branch renders cleanly.
	out := render.NewOutputKV().Add("k", "v").String()
	if out != "  k:  v\n" {
		t.Errorf("kv single line = %q", out)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// failMarshal fails both structured encoders so the renderer's marshal-error
// fallback is exercised deterministically.
type failMarshal struct{}

func (failMarshal) MarshalJSON() ([]byte, error) { return nil, errTest("json fail") }

func (failMarshal) MarshalYAML() (any, error) { return nil, errTest("yaml fail") }
