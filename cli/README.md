# cli

A terminal-UX toolkit for gokit command-line programs. It owns the command tree, presentation, input and cancellation concerns a CLI shares, so every gokit CLI parses, renders and behaves consistently. Commands use the standard `flag` package; no other parser is needed.

`cli` lives in the root module (`github.com/kbukum/gokit/cli`) and leans on the standard library, with `go.yaml.in/yaml/v3` (for `render`'s YAML output) as its only external dependency. It is a **light** Go CLI toolkit: theming, structured output, progress, prompts, signals, and a bounded live console. Heavy raw-mode rich widgets (arrow-key radio/checkbox lists) are out of scope by design.

> This is the one gokit module where writing to stdout/stderr is expected.
> Every renderer takes an injected `io.Writer`; every other module uses the injected logger.

## Packages

| Package | What it owns |
|---|---|
| [`command`](command) | `Program` and `Command`: subcommands, global flags, help with examples, typo suggestions, shell completions and usage exit codes |
| [`cienv`](cienv) | `Detect` reads the CI provider, repository, commit, branch and pipeline as labels |
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
	// Resolve styling once against the output stream's capability. TTY
	// detection is the caller's concern; wire it to your terminal check.
	stderrIsTTY := false
	palette := theme.PaletteForStream(theme.ColorAuto, stderrIsTTY)
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

## Commands

```go
var jsonOut bool
var out string
root := &command.Command{
	Name:  "tool",
	Flags: func(fs *flag.FlagSet) { fs.BoolVar(&jsonOut, "json", false, "Print JSON") },
	Commands: []*command.Command{{
		Name: "export", Summary: "Export a run", Args: "<run>",
		Flags:    func(fs *flag.FlagSet) { fs.StringVar(&out, "out", "", "Write to `file`") },
		Examples: []command.Example{{Description: "Save a run", Command: "tool export r1 --out r1.json"}},
		Run:      func(ctx context.Context, inv *command.Invocation) error { return export(ctx, inv.Args, out) },
	}},
}
err := command.Program{Root: root, Version: version, Stdout: os.Stdout, Stderr: os.Stderr}.Execute(ctx, os.Args[1:])
os.Exit(command.ExitCode(err).Int())
```

`tool completion bash|zsh|fish` prints a completion script; `tool help export` and `--help` print help. A typo such as `tool exprot` fails with `did you mean export?` and exit code 2.

## Determinism

Everything is testable without a real terminal:

- `prompt.ScriptedTerminal` replays canned input and captures output.
- `progress.Bar`/`progress.Spinner` advance only when the caller updates them — no clock, no background goroutine.
- `live.Console` redraws only on `Render()`, writing to an injected writer.

## Capability split (light by design)

gokit `cli` stays light: it lands theme/render/progress/prompt/signal plus a bounded live console. A full raw-mode rich TUI (live arrow-key navigation via a terminal driver dependency) is out of scope by design — the line-driven prompt path is always available and dependency-free.
