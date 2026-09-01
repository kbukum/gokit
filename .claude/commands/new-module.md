---
description: Scaffold a new package or module in the gokit multi-module monorepo the canonical way — decide root package vs sub-module, wire go.mod + replace directive, doc.go, domains.toml, and the right go.work file. Use when adding a new capability, package, or module to gokit, or when unsure whether new code belongs in the root module or its own go.mod.
---

# /new-module — router to the canonical skill

This command is a **thin router**. The single source of truth for this workflow is the
project skill at [`.github/skills/new-module/SKILL.md`](../../.github/skills/new-module/SKILL.md).

**Do this now:** read `.github/skills/new-module/SKILL.md` in full — plus every reference file it
links — and execute it exactly as written, applying it to the scope below. Do not act on any
summary; the skill file is authoritative and kept up to date. This router only exists so the
Claude Code slash command and the Copilot skill never drift.

Scope / arguments: $ARGUMENTS
