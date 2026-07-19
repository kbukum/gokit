# Pass 06 — Docs & supply chain

Docs drift and dependency risk are the quiet failures — the code works,
so nobody notices the stale godoc or the unvetted new dependency until much later.
This pass keeps the published surface honest and the dependency set clean.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* check the docs and deps the diff touches or invalidates.
*Project mode:* audit every module's `doc.go`, READMEs, `go.mod`/`go.sum`,
and the CI/release wiring for the invariants below.

## Docs

- **Public API documented.** Every exported identifier has a godoc comment starting with its name;
  every package has a `doc.go` with a package overview. gokit publishes to **pkg.go.dev**,
  so godoc *is* the public documentation — missing docs on new exports is a should-fix.
- **Docs match behavior.** A behavioral change updates the affected godoc, package overview,
  and any README/example. Stale docs that now describe removed or changed behavior are a should-fix.
  (Godoc *accuracy* vs the code is pass `07`; this check is that docs were updated at all.)
- **Canonical docs regenerated.** A module rename/add/remove updates `domains.toml`,
  `docs/MODULE-INDEX.md`, and `docs/parity-matrix.md` in the same change. A stale parity matrix
  or module index is a should-fix.
- **Examples compile.** Example code / `Example` test functions build and reflect the current API.
- **Prose flows naturally.** Markdown paragraphs stay continuous in source instead of being hard-wrapped to a fixed column; renderers own viewport-aware wrapping. `doc.go` and godoc/`//` prose likewise avoid arbitrary column-based breaks. Intentional paragraph, list, blockquote, table, and code-block structure remains intact. AI-generated hard wraps are a should-fix.

## Supply chain

- **New dep justified.** Each added dependency is necessary (stdlib does not already cover it — pass `01`),
  maintained (recent releases), license-compatible, and free of known advisories. An unjustified
  or unmaintained dependency is a should-fix; one with an open CVE is a blocker.
- **Vulnerability + license clean.** `govulncheck` passes;
  new findings are triaged in `.github/govulncheck-suppressions.json` with a rationale,
  not silently ignored. `gosec` (config in `gosec.toml`) passes.
- **Tidy modules.** `go.mod`/`go.sum` reflect exactly what is used (`go mod tidy` run per affected module);
  no leftover or phantom requires;
  `go.work` / `core.go.work` / `contrib.go.work` include any new module.
- **CI/release safety** (if touched). GitHub Actions pinned by commit SHA (never a moving tag);
  minimum job permissions; release artifacts signed (cosign) and SBOM (CycloneDX) produced.
  A workflow pinned to a tag or granting broad permissions is a should-fix.

## Detection starters

```bash
# exported identifiers missing a doc comment (spot-check the hits)
grep -rn --include=*.go '^func [A-Z]\|^type [A-Z]\|^var [A-Z]\|^const [A-Z]' . | grep -v _test.go
# packages missing doc.go
for d in $(find . -name '*.go' -not -name '*_test.go' | xargs -n1 dirname|sort -u); do \
  ls "$d"/doc.go >/dev/null 2>&1 || echo "no doc.go: $d"; done
# actions pinned by tag rather than SHA
grep -rn 'uses:' .github/workflows | grep -v '@[0-9a-f]\{40\}'
# module map / matrix touched when modules change?
git diff --name-only | grep -E 'domains.toml|MODULE-INDEX|parity-matrix'
```

## Validation gate

```bash
./gomod.sh cmd "go mod tidy" -m <module>          # per affected module
make lint                                          # includes vet; depguard for layering
govulncheck ./...                                  # vuln scan (from the affected module)
```

Docs updated alongside behavior, tidy modules, and a clean `govulncheck` pass this gate.
