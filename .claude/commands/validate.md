---
description: Build, vet, test, lint, tidy, and vuln-scan gokit changes through toven — the repo's argv-first task planner — scoped to the modules that actually changed. Use whenever you need to validate a gokit change, run tests for a package, reproduce CI locally, or check which modules an edit affects before committing.
---

# /validate — router to the canonical skill

This command is a **thin router**. The single source of truth for this workflow is the
project skill at [`.github/skills/validate/SKILL.md`](../../.github/skills/validate/SKILL.md).

**Do this now:** read `.github/skills/validate/SKILL.md` in full — plus every reference file it
links — and execute it exactly as written, applying it to the scope below. Do not act on any
summary; the skill file is authoritative and kept up to date. This router only exists so the
Claude Code slash command and the Copilot skill never drift.

Scope / arguments: $ARGUMENTS
