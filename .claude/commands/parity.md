---
description: Align gokit with its sibling kits (rskit, pykit) by capability, not blindly — mirror the strongest existing implementation for a given scope, keep gokit idiomatic Go, and keep docs/PARITY-MATRIX.md accurate. Use when porting or aligning a module with a sibling counterpart, deciding whether something should be shared or stay kit-only, or when touching anything that has a cross-kit parity row.
---

# /parity — router to the canonical skill

This command is a **thin router**. The single source of truth for this workflow is the
project skill at [`.github/skills/parity/SKILL.md`](../../.github/skills/parity/SKILL.md).

**Do this now:** read `.github/skills/parity/SKILL.md` in full — plus every reference file it
links — and execute it exactly as written, applying it to the scope below. Do not act on any
summary; the skill file is authoritative and kept up to date. This router only exists so the
Claude Code slash command and the Copilot skill never drift.

Scope / arguments: $ARGUMENTS
