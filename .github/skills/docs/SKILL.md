---
name: docs
description: >-
    Review and update gokit's documentation so it reads naturally and reflects the toolkit as it is
    today — keep Markdown paragraphs flowing without hard column wrapping, preserve intentional
    document structure, sync commands, module structure, and examples with the actual code, fix stale
    links and dead references, and drop history/plan narration. Use when writing or auditing docs,
    repairing AI-generated hard wraps, after a change that outdated docs, or before a release.
user-invocable: true
---

# Reviewing and updating gokit's docs

Documentation goes stale in two ways: it stops following the **style rules** (arbitrary source line breaks, history narration, dead links), and it becomes **outdated** (commands, module lists, package structure, and examples that no longer match the code). This skill sweeps both. gokit mirrors rskit capability by capability, so a stale doc misleads consumers and parity work alike — keep it accurate. Run it over the whole `docs/` tree, a single file, or the docs touched by a change set.

The authoritative doc policy lives in the Documentation section of [`.github/copilot-instructions.md`](../../copilot-instructions.md). The baseline wins over any local habit.

## The doc surface

Sweep every committed prose surface, not just `docs/`:

- `docs/**` — `PACKAGES.md`, `MODULE-INDEX.md`, `concern-owners.md`, `EXAMPLES.md`, `VERSIONING*.md`, `RELEASING.md`, `security-model.md`, `PARITY-MATRIX.md`, and the ADRs under `docs/adr/`.
- `README.md`, `CHANGELOG.md`, `MAINTAINERS.md`, and any top-level `*.md`.
- `.github/skills/**/SKILL.md` and their `references/*.md`.
- `doc.go` package documentation and `//` comments in the packages in scope (these are docs too).

Never touch `tmp/` (gitignored scratch) and never add a committed doc that references it.

## Pass 1 — Standards (how it reads)

- **Flowing Markdown prose.** A Markdown paragraph is one continuous source line. Do not hard-wrap prose to a column limit or add source newlines to control how it looks at one editor width; GitHub and other renderers wrap it for the reader's viewport. Collapse AI-generated hard wraps only within the same logical paragraph.
- **Preserve intentional structure.** Keep blank-line paragraph boundaries, headings, list items, blockquotes, tables, link definitions, HTML blocks, and fenced or indented code blocks. Never join separate list items or paragraphs. Preserve hard line breaks that are semantically meaningful (`<br>` or two trailing spaces).
- **Go documentation.** Write `doc.go`, godoc, and `//` prose naturally without arbitrary column-based breaks. Preserve Go directives, headings, lists, and indented code examples. Do not join separate comment paragraphs.
- **No history/plan/process narration.** A doc or comment describes the system as it is now, not how it got here or what a future plan intends. Delete "previously…", "we changed…", batch/plan/PR references, and TODO-narration.
- **`tmp/` stays uncommitted.** No committed doc references a `tmp/` plan or handoff note.
- **Frontmatter exemption.** YAML folded scalars (e.g. a skill's `description: >-`) already collapse to one logical line — leave their wrapping alone.

## Pass 2 — Up-to-date check (whether it's still true)

Verify each doc against the code it describes; a doc that lies is worse than no doc:

- **Commands & gates** match how the repo actually builds — the `Makefile` targets and the argv-first `toven` planner the repo drives tasks through (`make check`, `make lint M=<module>`, `make test M=<module>`, `make check-<domain>`, `make test-affected`). No renamed or removed target/verb lingers in the docs.
- **Module & package structure** matches reality: the module lists in `PACKAGES.md`/`MODULE-INDEX.md`, `go.work` membership, and the layer direction (`depguard`) match the tree; every package still has a `doc.go`; renamed/added/dropped modules are reflected everywhere they appear (including `concern-owners.md`).
- **Parity matrix** is current: `PARITY-MATRIX.md` reflects which rskit capabilities gokit currently mirrors (see the `parity` skill), including deliberate light-version or rskit-only decisions.
- **Examples build.** Code/command examples reflect current behavior and compile.
- **Links resolve.** Internal relative links and cross-references point at files that exist; other-repo references use full URLs, never bare `#123`.

## Apply, then validate

Fix every instance of a pattern across the whole surface in scope, not just the first hit. When repairing hard wraps, read and judge the Markdown structure rather than applying a blind line-joining script. Then validate what you touched:

```bash
go vet ./...                                        # validates the packages whose doc.go / comments changed compile
```

Docs/prose-only changes need no build/test gate beyond a `go vet`/`go build` of a package whose `doc.go` changed. Verify internal links by path before finishing.

## Commit

Use the [`commit`](../commit/SKILL.md) skill — one compact `docs:` Conventional-Commit line stating the change (e.g. `docs: repair hard-wrapped prose and sync PACKAGES.md`). No `Co-authored-by` trailer, no plan/batch/tool narration. Group by intent when it aids the reader (a prose-flow repair and an accuracy update read as separate commits).
