# Pass 07 — Comments & godoc semantics

The final pass, and a subtle one: comments and godoc are trusted by future readers and by pkg.go.dev, so a **wrong** comment is worse than no comment. Vibe-coded comments tend to narrate the obvious, restate the code, or describe what the code *used to* do. This pass keeps prose truthful and useful.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* read every comment and godoc line in the diff against the code beside it. *Project mode:* sample doc comments across packages, prioritizing public API godoc that pkg.go.dev renders.

## Checks

- **Godoc is accurate.** Each doc comment matches what the function/type actually does now — parameters, return values, error conditions, side effects, and any concurrency/ownership contract. A comment that describes the old behavior after a change is a should-fix (it will mislead every future reader and ship to pkg.go.dev).
- **Godoc convention.** Doc comments start with the identifier name (`// New returns …`, `// Cache is …`); package overview lives in `doc.go`. Exported items on the public surface are documented (ties to pass `06`; here the focus is *correctness* of the prose).
- **Comments earn their place.** Explain **why** — the non-obvious constraint, invariant,
  or trade-off — not **what** the code plainly says.
  Delete comments that merely restate the line below them.
- **No historical narration.** Comments describe the code as it is now, not the bug that was fixed
  or how it used to work. "Previously we… now we…" belongs in the commit message / git history,
  not the source. This is an explicit user preference — flag every instance.
- **No stale references.** Comments don't name removed types, old parameter names, dead flags,
  or moved files. TODO/FIXME/HACK carry a tracked issue link
  or they don't belong (ties to pass `04`).
- **Accurate, minimal.** When in doubt, fewer comments — but the ones present must be correct.
  A misleading comment is a should-fix even if the code is right.
- **Natural prose flow.** Comment and godoc prose is not hard-wrapped to a fixed column. Keep intentional paragraph, list, heading, directive, and code-example structure intact (details in pass `06`).

## Detection starters

Comment semantics are read-and-judge work; these only surface candidates.

```bash
# historical narration in comments (user-flagged anti-pattern)
grep -rn --include=*.go '//.*\(previously\|used to\|old \|before we\|now we\|changed to\|no longer\)' . | grep -v _test.go
# what-not-why restatement candidates: doc comment not starting with the identifier
grep -rn --include=*.go '^// [a-z]' . | grep -v _test.go
# stale-marker comments
grep -rn --include=*.go 'TODO\|FIXME\|HACK\|XXX' . | grep -v _test.go
```

Then read the doc comment on each changed exported identifier next to its implementation and confirm the prose is true. Accurate godoc that explains *why* passes; anything describing old behavior, restating the obvious, or narrating the fix does not.
