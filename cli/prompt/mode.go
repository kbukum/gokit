package prompt

// PromptMode is how a [Prompter] sources answers, resolved once up front.
//
// The decision models interactive stdio: prompts are read from stdin but rendered to stderr, so a session is interactive only when both streams are terminals. If stderr is redirected (cmd 2>log) the user would never see the question, so the mode falls back to [ModeNonInteractive] even when stdin is a TTY.
//
//nolint:revive // "PromptMode" reads clearly at call sites across sub-packages.
type PromptMode int

const (
	// ModeInteractive reads live, typed answers (both stdin and stderr are terminals). It is the zero value only nominally; callers resolve the mode explicitly via [ModeFromStdio].
	ModeInteractive PromptMode = iota
	// ModeNonInteractive never blocks (CI, piped, or a redirected prompt sink): each question resolves to its declared default.
	ModeNonInteractive
)

// ModeFromStdio resolves the mode from already-known stream TTY statuses, so it is environment-free and testable.
//
// Interactive only when both the input (stdin) and the prompt sink (stderr) are terminals, so a redirected sink never leaves the user blocked behind an invisible prompt.
func ModeFromStdio(stdinIsTTY, stderrIsTTY bool) PromptMode {
	if stdinIsTTY && stderrIsTTY {
		return ModeInteractive
	}
	return ModeNonInteractive
}

// IsInteractive reports whether this mode reads live answers.
func (m PromptMode) IsInteractive() bool {
	return m == ModeInteractive
}
