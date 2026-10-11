---
name: commit
description: "gokit: Commit the authorized staged change with a concise Conventional Commit message; only when asked."
user-invocable: true
---

# Commit an authorized change

1. Inspect `git status --short` and `git diff --cached`. Stage only authorized files; exclude secrets, generated junk, and scratch files.
2. Keep commits cohesive. Separate unrelated concerns unless the user requests a combined commit.
3. Write a short imperative subject, preferably under 72 characters, matching the repository's Conventional Commit style. Add a brief body only when the reason is not obvious.
4. Commit the reviewed staged change.

```bash
git commit -m "fix(fs): harden archive path validation"
```

Describe the change, not its execution history. No trailers, attributions, or plan/tool narration. Do not amend, rewrite history, or push without explicit authorization; never discard uncommitted work.
