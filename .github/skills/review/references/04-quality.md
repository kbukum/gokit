# Pass 04 — Quality, readability & maintainability

This is the pass the user cares about most: **is the code readable, maintainable, and well organized — or is it piled into one file?** Correctness passes (`02`, `03`) can be green while the code is still a maintenance liability. Vibe-coded output tends to grow one giant file with a few 300-line functions; this pass rejects that.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* judge the readability of the touched files and functions. *Project mode:* sweep for oversized files, god-packages, and duplicated logic across the tree.

## File & package organization (primary focus)

- **No piling into one file.** A package's functionality is split into focused files by concern —
  e.g. `client.go`, `options.go`, `errors.go`, `types.go`, `doc.go` —
  not one 800-line `<name>.go` holding everything. A single file that mixes types, construction,
  transport, and helpers is a **should-fix**; refactor into cohesive files.
- **One clear responsibility per file.** A reader should predict a file's contents from its name.
  Group related types + their methods together; keep unrelated concerns in separate files.
- **Cohesive packages.** A package is one concept.
  A grab-bag `utils`/`helpers` package accreting unrelated functions is a smell —
  place each helper with the concern it serves, or in the canonical owner (pass `01`).
- **Right-sized functions.** A function that does not fit on a screen
  or mixes several abstraction levels should be decomposed.
  Deeply nested conditionals → early returns / guard clauses. Prefer small,
  named helpers over inline complexity.

## Readability

- **Names reveal intent.** No cryptic abbreviations, no `data2`/`tmp3`, no misleading names.
  Exported identifiers read well at the call site (Go style: no stutter — `cache.New`, not `cache.NewCache`).
- **Straight-line where possible.** Minimize state and mutation;
  prefer clear sequential logic over clever one-liners. Complexity that must exist is isolated
  and named.
- **Errors add context.** Wrapped with `%w` and a message that says what failed,
  not a bare re-return that loses the call site.

## Maintainability

- **DRY within reason.** Copy-pasted blocks with small tweaks → one parameterized helper.
  (But do not over-abstract a single use.)
- **No dead or speculative code.** No commented-out blocks (git history exists), no unused exports,
  no "might need it later" scaffolding. Remove it.
- **Consistent with neighbors.** Matches the patterns of the surrounding package (option structs, constructor shape, error style) rather than introducing a one-off style.

## Signatures

- **Reduce parameters, don't wrap them.** Go has no column cap and `gofmt` does not wrap signatures, so a function taking 5+ positional parameters is a design signal: fold optional/defaulted or cohesive parameters into a typed request/options struct or functional options, using the owning package's existing `Config`/`...Option` pattern. `context.Context` stays first and is never folded into a struct. A genuinely distinct positional list may stay, but that is a recorded judgment, not the default.
- **No manual column-wrapping.** A signature hand-broken to hit a column is a fix-by-refactor,
  not an accepted state; the remedy is fewer parameters, not a wrapped parameter list.

## Detection starters

Read each hit — size and nesting are signals, not automatic verdicts.

```bash
# largest .go files (piling-into-one-file candidates)
find . -name '*.go' -not -name '*_test.go' | xargs wc -l | sort -rn | head -30
# packages that are a single big file (no concern split)
for d in $(find . -name '*.go' -not -name '*_test.go' | xargs -n1 dirname | sort -u); do \
  n=$(ls "$d"/*.go 2>/dev/null | grep -v _test.go | wc -l); echo "$n $d"; done | sort -n | head
# grab-bag packages
find . -type d \( -name utils -o -name helpers -o -name common -o -name misc \)
# commented-out code / stale markers
grep -rn --include=*.go '^\s*//\s*[a-z].*(' . | grep -v _test.go   # candidate commented-out lines
grep -rn --include=*.go 'TODO\|FIXME\|HACK' . | grep -v _test.go   # each needs a tracked issue link
```

Then run `make fmt` (`gofmt -s`) and `make lint` — a clean gofmt/golangci-lint run is necessary but not sufficient; the organization judgments above are the point of this pass.

## Output for this pass

For each finding, name the concrete refactor: which file to split, into which files, and along which concern boundary — so it is directly actionable, not just "this file is too big".
