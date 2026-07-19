---
name: commit
description: >-
    Commit staged (or explicitly named) changes with a single compact, developer-friendly commit
    message that states what changed — no Co-authored-by trailer, no plan/batch/PR numbers, no
    tool or review narration. Use when asked to commit work in gokit.
user-invocable: true
---

# Committing with a clean, developer-friendly message

A commit message tells the next developer *what changed and why*, in as few words as it takes. This skill keeps gokit's history tidy: one focused change per commit, a message that reads like a maintainer wrote it, and **nothing extra** — no trailers, no attributions, no process log.

## 1. Know what you're committing

```bash
git status --short          # what's staged vs. unstaged
git diff --cached           # the exact change going in
```

- Stage deliberately (`git add <paths>`); don't sweep in unrelated edits.
  If the working tree mixes concerns, make **separate commits**, one coherent change each.
- Don't commit generated junk, secrets, or `tmp/` scratch.

## 2. Write the message

A single compact subject line, imperative mood, describing the change **as it now stands**:

```
Add fix-reviews skill
Fix typos across skill docs
Rename Resolve to MustResolve in di container
```

- **Subject only** for small changes; keep it under ~72 chars. Add a short body (blank line, then wrapped prose) **only** when the *why* isn't obvious from the subject.
- Conventional-Commit prefixes (`feat:`, `fix:`, `docs:`) are fine when the repo/PR uses them — match the surrounding history; don't force them.
- Describe the current change, not the journey: no "previously we…", no "as requested in review", no bug-and-fix retelling.

**Never include:**

- a `Co-authored-by:` trailer or any other trailer/attribution,
- plan / batch / step / PR / issue numbers as scaffolding,
- tool, agent, or session references.

## 3. Commit

```bash
git commit -m "Add fix-reviews skill"
```

Use a message file for a subject + body:

```bash
git commit -F <path-to-message>
```

Do **not** amend or rewrite already-pushed history, and do **not** run destructive git commands on uncommitted work.
Push only when asked (or when the task — e.g. resolving PR reviews — requires the commit to be on the branch).
