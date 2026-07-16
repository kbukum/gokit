# cli

A parser-agnostic terminal-UX toolkit for gokit command-line programs. It is not
a flag parser; it owns the presentation, input, and cancellation concerns a CLI
shares, so every gokit CLI renders and behaves consistently.

`cli` lives in the root module (`github.com/kbukum/gokit/cli`) and leans on the
standard library, with `go.yaml.in/yaml/v3` (for `render`'s YAML output) as its
only external dependency. It is the **light** Go mirror of rskit's
`rskit-cli`: theming, structured output, progress, prompts, signals, and a
bounded live console. Heavy raw-mode rich widgets (arrow-key radio/checkbox
lists) stay rskit-only by design.

> This is the one gokit module where writing to stdout/stderr is expected. Every
> renderer takes an injected `io.Writer`; every other module uses the injected
> logger.

## Packages

| Package | What it owns |
|---|---|
| [`theme`](theme) | Semantic color `Palette` and status `Glyphs`, honoring `NO_COLOR`, TTY, and UTF-8 capability |
| [`render`](render) | `OutputTable`, `OutputKV`, `StatusReporter`, and `ErrorRenderer`/`ExitCode`/`OutputFormat` |
| [`progress`](progress) | Deterministic `Bar` and `Spinner` over an injected writer |
| [`prompt`](prompt) | `Prompter` with line + scripted terminals, non-interactive fallback, and validators |
| [`signal`](signal) | Interrupt handling as `context.Context` cancellation |
| [`live`](live) | A bounded multi-region live `Console` for concurrent streaming output |

## Quick start

```go
package main

import (
	"os"

	"github.com/kbukum/gokit/cli/render"
	"github.com/kbukum/gokit/cli/theme"
)

func main() {
	// Resolve styling once against the output stream's capability.
	palette := theme.PaletteForStream(theme.ColorAuto, isTTY(os.Stderr))
	status := render.NewStatusReporter(os.Stderr, palette, theme.GlyphsFromEnv())

	_ = status.Heading("Building")
	_ = status.Step(1, 2, "Compiling")
	_ = status.Success("Done")

	table := render.NewOutputTable("Name", "Count").
		AddRow("real", "500").
		AddRow("ai", "500")
	os.Stdout.WriteString(table.String() + "\n")
}
```

## Determinism

Everything is testable without a real terminal:

- `prompt.ScriptedTerminal` replays canned input and captures output.
- `progress.Bar`/`progress.Spinner` advance only when the caller updates them —
  no clock, no background goroutine.
- `live.Console` redraws only on `Render()`, writing to an injected writer.

## Capability split (light by design)

gokit `cli` mirrors rskit's *surface* idiomatically but stays light: it lands
theme/render/progress/prompt/signal plus a bounded live console. A full raw-mode
rich TUI (live arrow-key navigation via a terminal driver dependency) stays
rskit-only — the line-driven prompt path is always available and dependency-free.
