// Package cli is a parser-agnostic terminal-UX toolkit for building consistent
// command-line experiences across gokit services.
//
// It is not a flag parser; it owns the presentation, input, and cancellation
// concerns a CLI shares, split into focused sub-packages:
//
//   - [github.com/kbukum/gokit/cli/theme] — semantic color palette and status
//     glyphs honoring NO_COLOR, TTY detection, and UTF-8 capability.
//   - [github.com/kbukum/gokit/cli/render] — structured output: tables,
//     key-value blocks, status lines, and error/exit-code rendering.
//   - [github.com/kbukum/gokit/cli/progress] — deterministic progress bars and
//     spinners over an injected writer.
//   - [github.com/kbukum/gokit/cli/prompt] — interactive prompts (line terminal
//     plus a scripted test terminal) with a non-interactive fallback.
//   - [github.com/kbukum/gokit/cli/signal] — interrupt handling as
//     [context.Context] cancellation.
//   - [github.com/kbukum/gokit/cli/live] — a bounded multi-region live console
//     for concurrent streaming output.
//
// Rendering targets an injected io.Writer or returns a string the caller writes;
// this is the one gokit module where writing to stdout/stderr is expected —
// every other module uses the injected logger.
package cli
