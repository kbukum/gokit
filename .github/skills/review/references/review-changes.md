# Review changes

Standing, re-runnable review of a **change set** in this repository — a branch, a commit range, or `HEAD~1`. Use it after every change set, especially fast/"vibe-coded" work. It sequences the eight focused passes in [`references/`](./) over a diff and adds scope handling; the actual checks live in the focused files.

## Execution

Follow [the review skill](../SKILL.md): direct review by default; independent agents only on request. Read current source and relevant contracts. A plan is a scope checklist, not a justification for a baseline violation.

## Pass 0 — Scope and context

- Get the actual diff: `git diff <base>...HEAD --stat`, then per file. Review what changed **plus its blast radius** — the rest of each touched file, the code the change calls and is called by, and closely-related files in the same module. Do not audit the whole repo (that is [`review-project.md`](./review-project.md)), but do not tunnel-vision on the diff lines either.
- **Pre-existing problems in the blast radius are in scope.** A defect, dead code, duplicated concern, or design smell you read while reviewing is reported like any other finding — the change set is not a shield for the code around it. Because gokit is pre-stable with **no backward compatibility owed**, prefer a root-cause **redesign** over patching the symptom (decide Redesign / Align / Enhance / Drop; "leave it patched" is not an option). Flag when a fix reaches beyond the touched files and keep it coherent; never silently refactor unrelated code.
- gokit is a foundation toolkit: a change to a core package's public surface fans out to every sub-module, nested adapter, and downstream repo (rskit parity, consuming services). List that affected area before reviewing.
- Note whether the change belongs in the **root module**, a **sub-module** (own `go.mod`), or a **nested adapter** (e.g. `storage/s3`), and whether it belongs in *this* package at all.

## Passes

Follow the trigger table and order in [the review skill](../SKILL.md). Use each checklist's changes scope. Load applicable files only; report incomplete checks and stop acceptance on structural/reuse blockers.

## Findings

Record every finding as:

```
severity (blocker / should-fix / nit) — file:line — what's wrong — which principle — suggested fix
```

See [`SKILL.md`](../SKILL.md) for severity definitions.

## Validation

**Scope every command to the changed module(s) — do not run the full-tree gates here.** gokit has many modules; `make check` / `make test` / `make build` across the whole tree are slow and are reserved for [`review-project.md`](./review-project.md) or final pre-merge sign-off (typically in CI). For a change set, run only:

```bash
make fmt                             # gofmt -s clean
make lint M=<module>                 # golangci-lint, scoped
make test M=<module> T=<pattern>     # narrow further with a test pattern, -race -count=1
make test-affected                   # only modules the diff touches
make check-<domain>                  # per-domain gate if the change spans a domain
```

Use `./gomod.sh cmd "<command>" -m <module>` for a command in one module,
or `govulncheck ./...` in the touched module if a dependency changed.
Prefer `make test-affected` over the unscoped targets —
it runs only the modules impacted by the current changes.
Step up to a per-domain `make check-<domain>` when the change spans a domain.
Run the full `make check` only when the change is genuinely tree-wide,
or leave it to CI for sign-off. A green scoped run is necessary but **not sufficient** —
it will not catch goroutine leaks, missing timeouts/cancellation, unbounded channels,
global-registry composition smells, duplicated owners, or boundary- validation gaps.
Those are on the reviewer.
