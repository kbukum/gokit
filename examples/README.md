# gokit examples

Runnable demo programs that use gokit the way a downstream service would. They exist to show a capability end to end — wire it, run it, read the output — not to document an API surface. Each demo is a real consumer, so a primitive that has a demo here has a justified reason to exist and evolve.

This directory is a **separate Go workspace** (`examples/go.work`) that is deliberately kept out of the canonical workspaces (`go.work`, `core.go.work`, `contrib.go.work`). That isolation is the whole point:

- The example modules are **not published** and never enter the tagged release surface. Toven discovers release modules from the canonical workspace, so an example module — absent from it — is never listed by `toven modules` or a release plan.
- They stay out of `./...` product runs and coverage for the same reason.
- They are still **build- and vet-checked** through `make examples`, so they never rot.

## Conventions for a demo

Each demo is its own module under `examples/<name>/`:

- Its own `go.mod` with module path `github.com/kbukum/gokit/examples/<name>`.
- A `replace github.com/kbukum/gokit => ../..` directive so the demo builds against the working tree standalone, matching how gokit's sub-modules point back to the root.
- The module added to the `use (...)` block of `examples/go.work`.
- A declare-only `doc.go` holding the package clause and package documentation only; split real code into concern-named sibling files (`main.go`, the testable core, its test).
- No globals and no `init()` side effects: inject `context.Context`, logger, and any counter/meter into a testable core, keeping `main` a thin shell.

## Running

```bash
# Build and vet every demo (the gate that keeps them compiling):
make examples

# Run one demo:
(cd examples/<name> && go run .)

# Test one demo:
(cd examples/<name> && go test -race -count=1 -shuffle=on ./...)
```

## Demos

- [`broadcast-demo`](broadcast-demo/) — fans a stream of config-change events out to several subscribers with `stream.Broadcaster`, deliberately overruns a slow subscriber to show backpressure-by-drop, and bridges the broadcaster's `OnDrop` hook to an `observability` counter.
