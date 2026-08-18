---
description: Add a pluggable backend/adapter (storage, vectorstore, messaging, cache, llm, inference, …) to gokit the canonical way — a nested contrib sub-module with an explicit typed Register(registry, cfg) factory, no init() side effects, and the in-memory/local default kept in core. Use when integrating a provider like S3, GCS, Qdrant, Kafka, NATS, Redis, or a new LLM/inference provider into an existing gokit registry.
---

# /new-backend — router to the canonical skill

This command is a **thin router**. The single source of truth for this workflow is the
project skill at [`.github/skills/new-backend/SKILL.md`](../../.github/skills/new-backend/SKILL.md).

**Do this now:** read `.github/skills/new-backend/SKILL.md` in full — plus every reference file it
links — and execute it exactly as written, applying it to the scope below. Do not act on any
summary; the skill file is authoritative and kept up to date. This router only exists so the
Claude Code slash command and the Copilot skill never drift.

Scope / arguments: $ARGUMENTS
