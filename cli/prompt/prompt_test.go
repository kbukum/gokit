package prompt_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/prompt"
	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

func plainChoices() []prompt.Choice {
	return []prompt.Choice{
		prompt.NewChoice("go", "Go"),
		prompt.NewChoice("rust", "Rust").Recommended(),
		prompt.NewChoice("node", "Node.js"),
	}
}

func noDefaultChoices() []prompt.Choice {
	return []prompt.Choice{
		prompt.NewChoice("go", "Go"),
		prompt.NewChoice("rust", "Rust"),
	}
}

func linePrompter(term prompt.Terminal, mode prompt.PromptMode) *prompt.Prompter {
	return prompt.New(term, mode, theme.NewPalette(false))
}

// --- Choice ---

func TestChoiceAccessors(t *testing.T) {
	t.Parallel()
	c := prompt.NewChoice("rust", "Rust").WithAnnotation("detected").Recommended()
	if c.ID() != "rust" || c.Label() != "Rust" || c.Annotation() != "detected" || !c.IsRecommended() {
		t.Errorf("choice accessors wrong: %+v", c)
	}
	plain := prompt.NewChoice("go", "Go")
	if plain.Annotation() != "" || plain.IsRecommended() {
		t.Errorf("plain choice wrong: %+v", plain)
	}
}

// --- Mode ---

func TestModeFromStdio(t *testing.T) {
	t.Parallel()
	if prompt.ModeFromStdio(true, true) != prompt.ModeInteractive {
		t.Error("both TTY must be interactive")
	}
	for _, c := range [][2]bool{{true, false}, {false, true}, {false, false}} {
		if prompt.ModeFromStdio(c[0], c[1]) != prompt.ModeNonInteractive {
			t.Errorf("stdio %v must be non-interactive", c)
		}
	}
	if !prompt.ModeInteractive.IsInteractive() || prompt.ModeNonInteractive.IsInteractive() {
		t.Error("IsInteractive wrong")
	}
}

// --- Validators ---

func TestNonEmptyValidator(t *testing.T) {
	t.Parallel()
	v := prompt.NonEmpty("required")
	if err := v.Validate("value"); err != nil {
		t.Errorf("non-empty accepts value: %v", err)
	}
	if err := v.Validate("   "); err == nil || err.Error() != "required" {
		t.Errorf("blank must be rejected with message, got %v", err)
	}
}

func TestValidatorFuncAdapter(t *testing.T) {
	t.Parallel()
	v := prompt.ValidatorFunc(func(s string) error {
		if len(s) < 2 {
			return errString("too short")
		}
		return nil
	})
	if err := v.Validate("ok"); err != nil {
		t.Errorf("valid rejected: %v", err)
	}
	if err := v.Validate("x"); err == nil {
		t.Error("short must be rejected")
	}
}

// --- LineTerminal ---

func TestLineTerminalReadsTrimmedLinesThenEOF(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	term := prompt.NewLineTerminal(strings.NewReader("hello\nworld\n"), &out)
	for _, want := range []string{"hello", "world"} {
		line, ok, err := term.ReadLine()
		if err != nil || !ok || line != want {
			t.Fatalf("read = %q ok=%v err=%v, want %q", line, ok, err, want)
		}
	}
	if _, ok, _ := term.ReadLine(); ok {
		t.Error("expected EOF")
	}
}

func TestLineTerminalFinalLineWithoutNewline(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	term := prompt.NewLineTerminal(strings.NewReader("tail"), &out)
	line, ok, err := term.ReadLine()
	if err != nil || !ok || line != "tail" {
		t.Fatalf("read = %q ok=%v err=%v", line, ok, err)
	}
}

func TestLineTerminalWrites(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	term := prompt.NewLineTerminal(strings.NewReader(""), &out)
	if err := term.Write("a"); err != nil {
		t.Fatal(err)
	}
	if err := term.WriteLine("b"); err != nil {
		t.Fatal(err)
	}
	if err := term.Flush(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "ab\n" {
		t.Errorf("writes = %q", out.String())
	}
}

// --- ScriptedTerminal ---

func TestScriptedTerminalReplaysLinesThenEOF(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("one", "two")
	for _, want := range []string{"one", "two"} {
		line, ok, _ := term.ReadLine()
		if !ok || line != want {
			t.Fatalf("read = %q ok=%v, want %q", line, ok, want)
		}
	}
	if _, ok, _ := term.ReadLine(); ok {
		t.Error("expected EOF after script exhausted")
	}
}

func TestScriptedTerminalCapturesOutput(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal()
	_ = term.Write("x")
	_ = term.WriteLine("y")
	if term.Output() != "xy\n" {
		t.Errorf("output = %q", term.Output())
	}
}

// --- Confirm ---

func TestConfirmNonInteractiveUsesDefault(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	got, err := p.Confirm("proceed?", true)
	if err != nil || !got {
		t.Errorf("non-interactive confirm = %v err=%v", got, err)
	}
}

func TestConfirmParsesAnswers(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{"y": true, "yes": true, "n": false, "no": false, "": true}
	for input, want := range cases {
		term := prompt.NewScriptedTerminal().WithLine(input)
		p := linePrompter(term, prompt.ModeInteractive)
		got, err := p.Confirm("proceed?", true)
		if err != nil || got != want {
			t.Errorf("confirm(%q) = %v err=%v, want %v", input, got, err, want)
		}
	}
}

func TestConfirmReAsksOnGarbage(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("maybe", "y")
	p := linePrompter(term, prompt.ModeInteractive)
	got, err := p.Confirm("proceed?", false)
	if err != nil || !got {
		t.Fatalf("confirm = %v err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "'y' or 'n'") {
		t.Errorf("must warn on garbage: %q", term.Output())
	}
}

func TestConfirmClosedInputIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeInteractive)
	if _, err := p.Confirm("proceed?", true); err == nil {
		t.Error("closed input must error")
	}
}

// --- Text ---

func TestTextNonInteractiveResolvesDefault(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	def := "fallback"
	got, err := p.Text("name?", &def)
	if err != nil || got != "fallback" {
		t.Errorf("text = %q err=%v", got, err)
	}
}

func TestTextNonInteractiveNoDefaultIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	if _, err := p.Text("name?", nil); err == nil {
		t.Error("no default must error in non-interactive mode")
	}
}

func TestTextInteractiveReadsAndDefaults(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLine("typed")
	got, err := linePrompter(term, prompt.ModeInteractive).Text("name?", nil)
	if err != nil || got != "typed" {
		t.Errorf("text = %q err=%v", got, err)
	}

	def := "def"
	term2 := prompt.NewScriptedTerminal().WithLine("")
	got2, err := linePrompter(term2, prompt.ModeInteractive).Text("name?", &def)
	if err != nil || got2 != "def" {
		t.Errorf("blank must accept default, got %q err=%v", got2, err)
	}
}

func TestTextRequiredReAsksUntilNonEmpty(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("", "finally")
	got, err := linePrompter(term, prompt.ModeInteractive).Text("name?", nil)
	if err != nil || got != "finally" {
		t.Errorf("text = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "a value is required") {
		t.Errorf("must warn required: %q", term.Output())
	}
}

func TestTextWithValidatorReAsks(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("x", "long-enough")
	got, err := linePrompter(term, prompt.ModeInteractive).
		TextWith("name?", nil, prompt.ValidatorFunc(func(s string) error {
			if len(s) < 3 {
				return errString("too short")
			}
			return nil
		}))
	if err != nil || got != "long-enough" {
		t.Errorf("text = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "too short") {
		t.Errorf("must show rejection: %q", term.Output())
	}
}

func TestTextWithNonInteractiveRejectedDefaultIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	def := ""
	_, err := p.TextWith("name?", &def, prompt.NonEmpty("required"))
	if err == nil {
		t.Error("rejected default must be a typed error")
	}
	if _, ok := errors.AsAppError(err); !ok {
		t.Errorf("error must be AppError, got %T", err)
	}
}

// --- Select ---

func TestSelectNonInteractiveUsesRecommended(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	got, err := p.Select("lang?", plainChoices())
	if err != nil || got != "rust" {
		t.Errorf("select = %q err=%v", got, err)
	}
}

func TestSelectNonInteractiveNoRecommendedIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	if _, err := p.Select("lang?", noDefaultChoices()); err == nil {
		t.Error("no recommended default must error")
	}
}

func TestSelectEmptyChoicesIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeInteractive)
	if _, err := p.Select("lang?", nil); err == nil {
		t.Error("empty choices must error")
	}
}

func TestSelectInteractiveParsesNumberAndDefault(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLine("1")
	got, err := linePrompter(term, prompt.ModeInteractive).Select("lang?", plainChoices())
	if err != nil || got != "go" {
		t.Errorf("select = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "1) Go") || !strings.Contains(term.Output(), "(recommended)") {
		t.Errorf("numbered list wrong: %q", term.Output())
	}

	term2 := prompt.NewScriptedTerminal().WithLine("")
	got2, _ := linePrompter(term2, prompt.ModeInteractive).Select("lang?", plainChoices())
	if got2 != "rust" {
		t.Errorf("blank must accept recommended default, got %q", got2)
	}
}

func TestSelectReAsksOnInvalidNumber(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("9", "2")
	got, err := linePrompter(term, prompt.ModeInteractive).Select("lang?", plainChoices())
	if err != nil || got != "rust" {
		t.Errorf("select = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "between 1 and 3") {
		t.Errorf("must warn out of range: %q", term.Output())
	}
}

func TestSelectRequiredWhenNoDefault(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("", "1")
	got, err := linePrompter(term, prompt.ModeInteractive).Select("lang?", noDefaultChoices())
	if err != nil || got != "go" {
		t.Errorf("select = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "a choice is required") {
		t.Errorf("must warn required: %q", term.Output())
	}
}

func TestSelectRendersChoiceAnnotation(t *testing.T) {
	t.Parallel()
	choices := []prompt.Choice{
		prompt.NewChoice("go", "Go").WithAnnotation("detected"),
		prompt.NewChoice("rust", "Rust").Recommended(),
	}
	term := prompt.NewScriptedTerminal().WithLine("1")
	got, err := linePrompter(term, prompt.ModeInteractive).Select("lang?", choices)
	if err != nil || got != "go" {
		t.Fatalf("select = %q err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "detected") {
		t.Errorf("annotation must render beside the label: %q", term.Output())
	}
}

// --- MultiSelect ---

func TestMultiSelectNonInteractiveUsesRecommended(t *testing.T) {
	t.Parallel()
	choices := []prompt.Choice{
		prompt.NewChoice("a", "A").Recommended(),
		prompt.NewChoice("b", "B"),
		prompt.NewChoice("c", "C").Recommended(),
	}
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeNonInteractive)
	got, err := p.MultiSelect("pick?", choices)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("multi = %v", got)
	}
}

func TestMultiSelectInteractiveParsesList(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLine("1,3,3")
	choices := []prompt.Choice{
		prompt.NewChoice("a", "A"),
		prompt.NewChoice("b", "B"),
		prompt.NewChoice("c", "C"),
	}
	got, err := linePrompter(term, prompt.ModeInteractive).MultiSelect("pick?", choices)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("multi = %v (must dedupe)", got)
	}
}

func TestMultiSelectBlankAcceptsDefaults(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLine("")
	choices := []prompt.Choice{
		prompt.NewChoice("a", "A").Recommended(),
		prompt.NewChoice("b", "B"),
	}
	got, err := linePrompter(term, prompt.ModeInteractive).MultiSelect("pick?", choices)
	if err != nil || len(got) != 1 || got[0] != "a" {
		t.Errorf("multi = %v err=%v", got, err)
	}
}

func TestMultiSelectReAsksOnInvalid(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal().WithLines("9", "2")
	choices := []prompt.Choice{
		prompt.NewChoice("a", "A"),
		prompt.NewChoice("b", "B"),
	}
	got, err := linePrompter(term, prompt.ModeInteractive).MultiSelect("pick?", choices)
	if err != nil || len(got) != 1 || got[0] != "b" {
		t.Errorf("multi = %v err=%v", got, err)
	}
	if !strings.Contains(term.Output(), "comma-separated") {
		t.Errorf("must warn: %q", term.Output())
	}
}

func TestMultiSelectEmptyChoicesIsError(t *testing.T) {
	t.Parallel()
	p := linePrompter(prompt.NewScriptedTerminal(), prompt.ModeInteractive)
	if _, err := p.MultiSelect("pick?", nil); err == nil {
		t.Error("empty choices must error")
	}
}

// --- Prompter wiring ---

func TestPrompterExposesModeAndTerminal(t *testing.T) {
	t.Parallel()
	term := prompt.NewScriptedTerminal()
	p := linePrompter(term, prompt.ModeNonInteractive).WithGlyphs(theme.NewGlyphs(true))
	if p.Mode() != prompt.ModeNonInteractive {
		t.Error("mode wrong")
	}
	if p.Terminal() != term {
		t.Error("terminal wrong")
	}
}

func TestFromEnvNonInteractiveWhenNotTTY(t *testing.T) {
	t.Parallel()
	p := prompt.FromEnv(theme.ColorNever, false, false)
	if p.Mode() != prompt.ModeNonInteractive {
		t.Errorf("non-TTY env must be non-interactive")
	}
}

func TestLineTerminalWritePropagatesError(t *testing.T) {
	t.Parallel()
	term := prompt.NewLineTerminal(strings.NewReader(""), failWriter{})
	if err := term.Write("x"); err == nil {
		t.Error("Write must surface a writer failure")
	}
	if err := term.WriteLine("x"); err == nil {
		t.Error("WriteLine must surface a writer failure")
	}
}

func TestLineTerminalFlushPropagatesError(t *testing.T) {
	t.Parallel()
	term := prompt.NewLineTerminal(strings.NewReader(""), flushErrWriter{})
	if err := term.Flush(); err == nil {
		t.Error("Flush must surface a flusher failure")
	}
}

func TestLineTerminalReadPropagatesError(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	term := prompt.NewLineTerminal(failReader{}, &out)
	if _, _, err := term.ReadLine(); err == nil {
		t.Error("ReadLine must surface a reader failure")
	}
}

func TestPromptsSurfaceReadErrors(t *testing.T) {
	t.Parallel()
	newErr := func() *prompt.Prompter {
		return linePrompter(&readErrTerminal{}, prompt.ModeInteractive)
	}
	if _, err := newErr().Confirm("go?", true); err == nil {
		t.Error("confirm must surface a read error")
	}
	def := "d"
	if _, err := newErr().Text("name?", &def); err == nil {
		t.Error("text must surface a read error")
	}
	if _, err := newErr().Select("pick?", plainChoices()); err == nil {
		t.Error("select must surface a read error")
	}
	if _, err := newErr().MultiSelect("pick?", plainChoices()); err == nil {
		t.Error("multi-select must surface a read error")
	}
}

func TestPromptsSurfaceWriteErrors(t *testing.T) {
	t.Parallel()
	newErr := func() *prompt.Prompter {
		return linePrompter(&writeErrTerminal{}, prompt.ModeInteractive)
	}
	if _, err := newErr().Confirm("go?", true); err == nil {
		t.Error("confirm must surface a write error")
	}
	def := "d"
	if _, err := newErr().Text("name?", &def); err == nil {
		t.Error("text must surface a write error")
	}
	if _, err := newErr().Select("pick?", plainChoices()); err == nil {
		t.Error("select must surface a write error")
	}
	if _, err := newErr().MultiSelect("pick?", plainChoices()); err == nil {
		t.Error("multi-select must surface a write error")
	}
}

func TestPromptsSurfaceFlushErrorsAtAnswerMarker(t *testing.T) {
	t.Parallel()
	// Heading/list writes succeed; the answer-marker Flush (first flush) fails.
	if _, err := linePrompter(&gateTerminal{flushAt: 1}, prompt.ModeInteractive).
		Confirm("go?", true); err == nil {
		t.Error("confirm must surface the flush failure")
	}
	def := "d"
	if _, err := linePrompter(&gateTerminal{flushAt: 1}, prompt.ModeInteractive).
		Text("name?", &def); err == nil {
		t.Error("text must surface the answer-marker flush failure")
	}
	if _, err := linePrompter(&gateTerminal{flushAt: 1}, prompt.ModeInteractive).
		Select("pick?", plainChoices()); err == nil {
		t.Error("select must surface the answer-marker flush failure")
	}
	if _, err := linePrompter(&gateTerminal{flushAt: 1}, prompt.ModeInteractive).
		MultiSelect("pick?", plainChoices()); err == nil {
		t.Error("multi-select must surface the answer-marker flush failure")
	}
}

func TestPromptsSurfaceNoticeWriteErrors(t *testing.T) {
	t.Parallel()
	// A garbage answer drives the re-ask notice; that WriteLine is gated to fail.
	confirm := &gateTerminal{lines: []string{"maybe"}, writeLnAt: 1}
	if _, err := linePrompter(confirm, prompt.ModeInteractive).Confirm("go?", true); err == nil {
		t.Error("confirm must surface the notice write error")
	}
	// select/multi print heading + one WriteLine per choice, so the notice is the
	// (2+len(choices))th WriteLine.
	noticeAt := 2 + len(plainChoices())
	sel := &gateTerminal{lines: []string{"99"}, writeLnAt: noticeAt}
	if _, err := linePrompter(sel, prompt.ModeInteractive).Select("pick?", plainChoices()); err == nil {
		t.Error("select must surface the notice write error")
	}
	multi := &gateTerminal{lines: []string{"99"}, writeLnAt: noticeAt}
	if _, err := linePrompter(multi, prompt.ModeInteractive).MultiSelect("pick?", plainChoices()); err == nil {
		t.Error("multi-select must surface the notice write error")
	}
	// text prints the heading then the required notice: the 2nd WriteLine.
	txt := &gateTerminal{lines: []string{""}, writeLnAt: 2}
	if _, err := linePrompter(txt, prompt.ModeInteractive).Text("name?", nil); err == nil {
		t.Error("text must surface the required-notice write error")
	}
}

func TestPromptsSurfaceRowWriteErrors(t *testing.T) {
	t.Parallel()
	// Heading WriteLine succeeds; the first choice-row WriteLine (2nd) fails.
	sel := &gateTerminal{lines: []string{"1"}, writeLnAt: 2}
	if _, err := linePrompter(sel, prompt.ModeInteractive).Select("pick?", plainChoices()); err == nil {
		t.Error("select must surface a row write error")
	}
	multi := &gateTerminal{lines: []string{"1"}, writeLnAt: 2}
	if _, err := linePrompter(multi, prompt.ModeInteractive).MultiSelect("pick?", plainChoices()); err == nil {
		t.Error("multi-select must surface a row write error")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// failWriter always fails writes.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errString("write failed") }

// flushErrWriter accepts writes but fails Flush, driving the flush error path.
type flushErrWriter struct{}

func (flushErrWriter) Write(p []byte) (int, error) { return len(p), nil }

func (flushErrWriter) Flush() error { return errString("flush failed") }

// failReader always fails reads.
type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errString("read failed") }

// readErrTerminal fails on ReadLine but succeeds on writes, so prompts reach
// their read step before failing.
type readErrTerminal struct{}

func (readErrTerminal) ReadLine() (line string, ok bool, err error) {
	return "", false, errString("read failed")
}
func (readErrTerminal) Write(string) error     { return nil }
func (readErrTerminal) WriteLine(string) error { return nil }
func (readErrTerminal) Flush() error           { return nil }

// writeErrTerminal fails on the first write, so prompts surface a write failure
// before any input is read.
type writeErrTerminal struct{}

func (writeErrTerminal) ReadLine() (line string, ok bool, err error) { return "", false, nil }
func (writeErrTerminal) Write(string) error                          { return errString("write failed") }
func (writeErrTerminal) WriteLine(string) error                      { return errString("write failed") }
func (writeErrTerminal) Flush() error                                { return errString("flush failed") }

// gateTerminal replays scripted input and fails a chosen write family on its
// Nth call, so deep write-error branches (choice rows, the answer-marker flush,
// and re-ask notices) can be reached deterministically. A zero target never
// fails.
type gateTerminal struct {
	lines      []string
	cursor     int
	writeAt    int
	writeLnAt  int
	flushAt    int
	writes     int
	writeLines int
	flushes    int
}

func (t *gateTerminal) ReadLine() (line string, ok bool, err error) {
	if t.cursor >= len(t.lines) {
		return "", false, nil
	}
	line = t.lines[t.cursor]
	t.cursor++
	return line, true, nil
}

func (t *gateTerminal) Write(string) error {
	t.writes++
	return failAt(t.writeAt, t.writes)
}

func (t *gateTerminal) WriteLine(string) error {
	t.writeLines++
	return failAt(t.writeLnAt, t.writeLines)
}

func (t *gateTerminal) Flush() error {
	t.flushes++
	return failAt(t.flushAt, t.flushes)
}

// failAt fails once the call count reaches a non-zero target, and on every call after.
func failAt(target, count int) error {
	if target != 0 && count >= target {
		return errString("gated")
	}
	return nil
}
