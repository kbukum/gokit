// Package render is the structured, non-interactive terminal display layer of
// the CLI kit.
//
// It covers everything a command emits when it is reporting rather than
// prompting:
//
//   - [OutputTable] — aligned tables for row/column data.
//   - [OutputKV] — key-value blocks for headers and summaries.
//   - [OutputFormat], the shared [ExitCode] convention, and an [ErrorRenderer]
//     that turns an [github.com/kbukum/gokit/errors.AppError] into consistent
//     text/JSON/YAML.
//   - [StatusReporter] — one-off feedback lines (success/warn/step/heading) for
//     guided, multi-step flows.
//
// Every renderer writes to an injected [io.Writer] and composes the theme layer
// ([github.com/kbukum/gokit/cli/theme.Palette] and
// [github.com/kbukum/gokit/cli/theme.Glyphs]), so color and symbols honor
// NO_COLOR, TTY detection, and UTF-8 capability uniformly.
package render
