// Package prompt provides interactive prompts for guided CLI flows.
//
// It asks a question — a yes/no confirm, freeform text, a single [Choice], or a multi-select — and returns a typed answer, reusing the theme layer so styling honors NO_COLOR, TTY, and UTF-8 detection like the rest of the CLI kit.
//
// Prompts speak through a [Terminal] seam rather than raw stdio, so the same question renders as a numbered list over a pipe ([LineTerminal]) or replays a canned sequence in a test ([ScriptedTerminal]) without the calling code changing.
//
// # Non-interactive fallback
//
// A [Prompter] resolves its behavior once from a [PromptMode]:
//
//   - [ModeInteractive] renders prompts and reads answers.
//   - [ModeNonInteractive] never blocks: each question resolves to its declared
//     default (the recommended [Choice] for a selection, the supplied default
//     for a confirm/text). A required question with no default returns a typed
//     [github.com/kbukum/gokit/errors.AppError] rather than an invented answer or
//     a hang.
package prompt
