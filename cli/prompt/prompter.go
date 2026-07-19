package prompt

import (
	"github.com/kbukum/gokit/cli/theme"
)

// Prompter is a terminal-agnostic prompt driver.
//
// It binds a [Terminal], a resolved [PromptMode], and a rendering [Style],
// and exposes one method per question type. The same call works over cooked stdio
// or a scripted test double. Build one from explicit parts with [New] (deterministic tests)
// or from the environment with [FromEnv].
type Prompter struct {
	terminal Terminal
	mode     PromptMode
	style    Style
}

// New builds a prompter from an explicit terminal, mode, and palette.
//
// Glyphs default to the ASCII fallback for byte-clean, deterministic tests;
// override with [Prompter.WithGlyphs].
func New(terminal Terminal, mode PromptMode, palette theme.Palette) *Prompter {
	return &Prompter{
		terminal: terminal,
		mode:     mode,
		style:    NewStyle(palette, theme.NewGlyphs(false)),
	}
}

// FromEnv builds a prompter bound to process stdin/stderr.
//
// The [PromptMode] follows whether both stdin and stderr are terminals,
// and the [theme.Palette] follows color against stderr, so interactivity
// and styling both honor redirection and NO_COLOR. Prompts render to stderr,
// so a redirected stderr forces [ModeNonInteractive] rather than blocking on input behind an invisible prompt.
func FromEnv(color theme.ColorChoice, stdinIsTTY, stderrIsTTY bool) *Prompter {
	mode := ModeFromStdio(stdinIsTTY, stderrIsTTY)
	palette := theme.PaletteForStream(color, stderrIsTTY)
	p := New(NewStdioTerminal(), mode, palette)
	p.style = NewStyle(palette, theme.GlyphsFromEnv())
	return p
}

// WithGlyphs overrides the glyph set (Unicode symbols vs ASCII fallback) and returns the receiver.
func (p *Prompter) WithGlyphs(glyphs theme.Glyphs) *Prompter {
	p.style = NewStyle(p.style.Palette(), glyphs)
	return p
}

// Mode returns the resolved interaction mode.
func (p *Prompter) Mode() PromptMode { return p.mode }

// Terminal returns the bound terminal, for inspecting captured output in tests.
func (p *Prompter) Terminal() Terminal { return p.terminal }

// session binds the terminal, style, and mode threaded through every prompt.
type session struct {
	term  Terminal
	style Style
	mode  PromptMode
}

func (p *Prompter) session() session {
	return session{term: p.terminal, style: p.style, mode: p.mode}
}

// Select asks for exactly one choice.
//
// In [ModeNonInteractive] it resolves to the recommended choice; with none it is a typed error.
// Interactively it shows a numbered list.
func (p *Prompter) Select(prompt string, choices []Choice) (ChoiceID, error) {
	return p.session().runSelect(prompt, choices)
}

// MultiSelect asks for zero or more choices.
//
// The default answer is the set of recommended choices, which may be empty.
// Interactively it accepts a comma-separated list of numbers.
func (p *Prompter) MultiSelect(prompt string, choices []Choice) ([]ChoiceID, error) {
	return p.session().runMultiSelect(prompt, choices)
}

// Confirm asks a yes/no question with an explicit default.
func (p *Prompter) Confirm(prompt string, def bool) (bool, error) {
	return p.session().runConfirm(prompt, def)
}

// Text asks for freeform text with an optional default.
//
// A nil def means no default: in [ModeNonInteractive] that is a typed error.
func (p *Prompter) Text(prompt string, def *string) (string, error) {
	value, has := deref(def)
	return p.session().runText(prompt, value, has, nil)
}

// TextWith asks for freeform text validated by validator, re-asking on rejection.
//
// In [ModeNonInteractive] a rejected default is a typed error rather than a silent bad value.
func (p *Prompter) TextWith(prompt string, def *string, validator Validator) (string, error) {
	value, has := deref(def)
	return p.session().runText(prompt, value, has, validator)
}

func deref(s *string) (string, bool) {
	if s == nil {
		return "", false
	}
	return *s, true
}
