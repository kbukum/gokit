---
name: docs
description: "gokit: Update or audit documentation for accuracy, clear prose, working examples, and links."
user-invocable: true
---

# Documentation

Apply [Documentation and consumer experience](../../engineering.md#documentation-and-consumer-experience) and the [guidance map](../../engineering.md#where-guidance-lives). Scope to the requested docs and directly affected references, including godoc and agent docs; do not sweep unrelated files.

1. **Verify facts.** Check commands against `make help` and Toven, modules against `go.mod`/`go.work`, ownership against `docs/concern-owners.md`, and APIs against source. Never claim a rule is enforced unless a gate checks it; see [Enforcement](../../engineering.md#enforcement). Use the parity skill only for parity claims.
2. **Write for the reader.** Lead with the capability or task, then a working example for how-to pages. Use short active sentences and focused headings. Keep configuration, contracts, safety requirements, and verification details where needed; remove repeated rationale, implementation narration, and unnecessary API inventories. Changelogs use high-level outcome summaries.
3. **Preserve structure.** Markdown and Go comment prose have no arbitrary column wrapping. Keep paragraphs, lists, code, directives, HTML, and meaningful hard breaks distinct; do not blindly join lines.
4. **Remove stale guidance.** Describe current behavior, not implementation history. Preserve historical changelogs and accepted ADRs. Do not link stable docs to temporary plans.
5. **Check the result.** Resolve relative links and anchors. Compile changed executable examples; use scoped `go vet` for changed godoc source. Prose-only edits do not require an application build.

For instruction/skill edits, keep always-loaded rules short. Each topic has one owner in the [guidance map](../../engineering.md#where-guidance-lives); link it instead of restating it. A description states when to use the skill; the body states actions and acceptance. Preserve hard requirements and check metadata/link validity.

Commit only if explicitly requested, using the commit skill.
