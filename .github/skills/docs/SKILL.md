---
name: docs
description: "gokit: Update or audit documentation for accuracy, clear prose, working examples, and links."
user-invocable: true
---

# Documentation

Apply the [baseline](../../copilot-instructions.md). Scope to the requested docs and directly affected references, including godoc and agent docs; do not sweep unrelated files.

1. **Verify facts.** Check commands against Makefile/Toven, modules against `go.mod`/`go.work`, ownership against `docs/concern-owners.md`, and APIs against source. Use the parity skill only for parity claims.
2. **Write for the task.** Lead how-to pages with a working example. Use plain active sentences, focused headings, and tables for dense options. Add a small captioned Mermaid diagram only when it replaces harder prose.
3. **Preserve structure.** Markdown and Go comment prose have no arbitrary column wrapping. Keep paragraphs, lists, code, directives, HTML, and meaningful hard breaks distinct; do not blindly join lines.
4. **Remove stale guidance.** Describe current behavior, not implementation history. Preserve historical changelogs and accepted ADRs. Do not link stable docs to temporary plans.
5. **Check the result.** Resolve relative links and anchors. Compile changed executable examples; use scoped `go vet` for changed godoc source. Prose-only edits do not require an application build.

For instruction/skill edits, keep always-loaded rules short and self-contained. A description states when to use the skill; the body states actions and acceptance. Link task-specific detail without recursively loading it. Preserve hard requirements and check metadata/link validity.

Commit only if explicitly requested, using the commit skill.
