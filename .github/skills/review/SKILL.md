---
name: review
description: "gokit: Review a diff, package, or tree for placement, runtime safety, public API, tests, quality, docs, and gates."
user-invocable: true
---

# Review

Review code and evidence against the [engineering standard](../../engineering.md), not the author's rationale. Plans define scope, not exceptions to the rules.

Review directly. Launch an independent reviewer only when the user asks; pass it the scope, constraints, and files, not the authoring transcript. Re-check returned findings against source before acting.

Reviews are read-only unless fixes are authorized. Preserve user edits and the index. Never commit, amend, push, resolve threads, or post replies unless asked.

## Modes

**Changes** — a diff, branch, or commit range.

1. Get the diff (`git diff <base>...HEAD --stat`, plus staged, unstaged, and untracked work when asked).
2. List the impact area: touched files, close callers and callees, and affected contracts. A core public-surface change reaches every sub-module and consumer.
3. Apply each lens whose trigger matches, in the order below. Report pre-existing defects inside the impact area; do not widen into an unrelated audit.
4. Run scoped gates only: `make lint M=<module>`, `make test M=<module> T=<pattern>`, `make test-affected`, or `make check-<domain>`.

**Project** — a package, domain, or the whole tree.

1. State the boundary first (one package, one domain from `toven groups`, or everything).
2. Map it: `toven modules`, `toven groups`, and each module's gokit requires (`grep -E '^\s+github.com/kbukum/gokit' <module>/go.mod`).
3. Apply every lens in priority order. Group findings by module, then by lens.
4. Run the full gates for the boundary: `make check`, `make lint`, `make test-coverage`, `make vuln`, `make licenses`, `make tidy-check`, `make structure`.

**Review and fix** — only when fixes are requested.

1. Run the changes or project mode first and share the findings.
2. Confirm the fix scope when it is unclear. For each fix choose Redesign, Align, Enhance, or Drop with a one-line reason.
3. Fix test first, rerun the affected gates, and report what is still open.
4. A fix request is not permission to commit, push, or publish.

## Lenses

Apply in this order. A blocker in placement and ownership stops acceptance. Load only the lens files whose trigger matches; for prose-only changes use docs and gates only.

| Order | Lens | Trigger |
|---|---|---|
| 1 | [Placement and ownership](references/placement-ownership.md) | New or moved code, imports, `go.mod`, shared helpers |
| 2 | [Runtime and security](references/runtime-security.md) | Errors, goroutines, registries, remote calls, input, auth, crypto, AI |
| 3 | [Public API and stability](references/public-api.md) | Exported identifiers, signatures, options, breaking changes |
| 4 | [Tests and proof](references/tests-proof.md) | Any behavior change, tests, fixtures, coverage |
| 5 | [Code quality](references/code-quality.md) | File layout, readability, dead code, comments |
| 6 | [Docs and supply chain](references/docs-supply-chain.md) | Docs, godoc, dependencies, CI, release |
| 7 | [Gates](references/gates.md) | Claims about enforcement, gate config, or before acceptance |

gokit has no UI, so there is no UI lens.

## Severity

| Severity | Meaning | Gates acceptance |
|---|---|---|
| **blocker** | Broken behavior or contract, a security hole, or a hard-rule violation (misplaced code, forked owner, runtime panic, leaked goroutine, untested behavior) | Yes |
| **should-fix** | A real maintainability, docs, or test defect with clear evidence | Fix in this change unless the user defers it |
| **nit** | Optional style | Never |

## Findings

One line per finding:

```
<severity> | <file:line> | <lens> | <defect and impact> | <Redesign|Align|Enhance|Drop>: <fix>
```

Group repeated patterns into one finding with every location. Detection commands find candidates, not verdicts; read each hit. List skipped lenses, unrun gates, and uncertainty. Green gates are necessary, not proof of lifecycle or security correctness.
