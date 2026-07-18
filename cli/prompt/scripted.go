package prompt

import "strings"

// ScriptedTerminal is a deterministic in-memory [Terminal] for tests and examples.
//
// It is a first-class injectable double, not a test-only escape hatch:
// it replays a canned sequence of input lines and records everything written,
// so the line-driven prompt path can be exercised without a real terminal.
// Queue input with [ScriptedTerminal.WithLine] / [ScriptedTerminal.WithLines],
// then read back rendered output via [ScriptedTerminal.Output].
type ScriptedTerminal struct {
	inputs []string
	cursor int
	output strings.Builder
}

// NewScriptedTerminal creates an empty scripted terminal.
func NewScriptedTerminal() *ScriptedTerminal {
	return &ScriptedTerminal{}
}

// WithLine queues one input line and returns the receiver.
func (t *ScriptedTerminal) WithLine(line string) *ScriptedTerminal {
	t.inputs = append(t.inputs, line)
	return t
}

// WithLines queues several input lines in order and returns the receiver.
func (t *ScriptedTerminal) WithLines(lines ...string) *ScriptedTerminal {
	t.inputs = append(t.inputs, lines...)
	return t
}

// Output returns everything written to the terminal so far, for assertions.
func (t *ScriptedTerminal) Output() string {
	return t.output.String()
}

// ReadLine returns the next queued line; ok is false once the script is exhausted (end of input).
func (t *ScriptedTerminal) ReadLine() (line string, ok bool, err error) {
	if t.cursor >= len(t.inputs) {
		return "", false, nil
	}
	line = t.inputs[t.cursor]
	t.cursor++
	return line, true, nil
}

// Write records text verbatim.
func (t *ScriptedTerminal) Write(text string) error {
	t.output.WriteString(text)
	return nil
}

// WriteLine records text followed by a newline.
func (t *ScriptedTerminal) WriteLine(text string) error {
	t.output.WriteString(text)
	t.output.WriteByte('\n')
	return nil
}

// Flush is a no-op for the in-memory terminal.
func (t *ScriptedTerminal) Flush() error { return nil }

// compile-time assurance the double satisfies the seam.
var _ Terminal = (*ScriptedTerminal)(nil)
