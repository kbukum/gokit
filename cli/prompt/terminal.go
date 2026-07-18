package prompt

import (
	"bufio"
	stderrors "errors"
	"io"
	"os"
	"strings"

	"github.com/kbukum/gokit/errors"
)

// Terminal is the interactive medium a [Prompter] reads from and renders to.
//
// It abstracts how a prompt reads a line of input and writes output — decoupled from what a prompt asks — so one set of prompt-kind logic drives both real cooked stdio ([LineTerminal]) and a deterministic test double ([ScriptedTerminal]).
type Terminal interface {
	// ReadLine reads one whole line. The second return value is false at end of input, so callers surface a typed "input closed" error instead of hanging.
	ReadLine() (line string, ok bool, err error)
	// Write writes text verbatim (no trailing newline).
	Write(text string) error
	// WriteLine writes text followed by a newline.
	WriteLine(text string) error
	// Flush flushes any buffered output.
	Flush() error
}

// LineTerminal is a cooked-stdio [Terminal] over an injected reader and writer.
//
// The user types a whole line and presses Enter; it never enters raw mode, works over pipes, and needs no extra dependencies, so it is the default terminal. Bind it to real streams with [NewStdioTerminal], or to in-memory buffers with [NewLineTerminal].
type LineTerminal struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewLineTerminal builds a line terminal from an explicit reader and writer.
func NewLineTerminal(r io.Reader, w io.Writer) *LineTerminal {
	return &LineTerminal{reader: bufio.NewReader(r), writer: w}
}

// NewStdioTerminal builds a line terminal bound to process stdin and stderr, following the "prompts to stderr" convention so piped stdout stays clean.
func NewStdioTerminal() *LineTerminal {
	return NewLineTerminal(os.Stdin, os.Stderr)
}

// ReadLine reads a line, trimming the trailing newline; ok is false at EOF.
func (t *LineTerminal) ReadLine() (line string, ok bool, err error) {
	raw, err := t.reader.ReadString('\n')
	if err != nil {
		if stderrors.Is(err, io.EOF) {
			if raw == "" {
				return "", false, nil
			}
			// A final line without a trailing newline is still a valid answer.
			return strings.TrimRight(raw, "\r\n"), true, nil
		}
		return "", false, errors.Internal(err)
	}
	return strings.TrimRight(raw, "\r\n"), true, nil
}

// Write writes text with no trailing newline.
func (t *LineTerminal) Write(text string) error {
	if _, err := io.WriteString(t.writer, text); err != nil {
		return errors.Internal(err)
	}
	return nil
}

// WriteLine writes text followed by a newline.
func (t *LineTerminal) WriteLine(text string) error {
	if _, err := io.WriteString(t.writer, text+"\n"); err != nil {
		return errors.Internal(err)
	}
	return nil
}

// Flush flushes the underlying writer when it supports flushing.
func (t *LineTerminal) Flush() error {
	if f, ok := t.writer.(interface{ Flush() error }); ok {
		if err := f.Flush(); err != nil {
			return errors.Internal(err)
		}
	}
	return nil
}
