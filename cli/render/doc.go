// Package render is the structured, non-interactive terminal display layer of
// the CLI kit.
//
// It covers everything a command emits when it is reporting rather than
// prompting:
//
//   - [OutputTable] — aligned tables for row/column data, returned as a string.
//   - [OutputKV] — key-value blocks for headers and summaries, returned as a
//     string.
//   - [OutputFormat], the shared [ExitCode] convention, and an [ErrorRenderer]
//     that turns an [github.com/kbukum/gokit/errors.AppError] into consistent
//     text/JSON/YAML.
//   - [StatusReporter] — one-off feedback lines (success/warn/step/heading) for
//     guided, multi-step flows, written to an injected [io.Writer].
//
// The pure builders ([OutputTable], [OutputKV], [ErrorRenderer]) return strings
// the caller writes; [StatusReporter] writes to an injected [io.Writer]. Theme
// use is per renderer: [StatusReporter] applies the palette and glyphs
// ([github.com/kbukum/gokit/cli/theme.Palette],
// [github.com/kbukum/gokit/cli/theme.Glyphs]) so color and symbols honor
// NO_COLOR, TTY detection, and UTF-8 capability; [OutputTable] selects its
// border charset from glyph capability; [OutputKV] and [ErrorRenderer] emit
// plain text.
package render
