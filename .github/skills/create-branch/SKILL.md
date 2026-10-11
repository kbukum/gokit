---
name: create-branch
description: "gokit: Create a change-named kbukum/ branch from an up-to-date main when starting new work."
user-invocable: true
---

# Create a branch

1. Inspect Git state and preserve existing edits and staging. Confirm which changes belong on the branch.
2. Fetch the remote and branch from the latest `origin/main`; use another base only when explicitly requested.
3. Name the branch `kbukum/<short-change-summary>`, using lowercase kebab-case.

```bash
git status --short
git fetch origin
git switch -c kbukum/<change-name> origin/main
```

Name the capability or fix, not a plan, batch, step, session, or machine. Keep the branch understandable as a standalone change.

Uncommitted work may follow the switch; if it conflicts, stop rather than discard it. This skill does not authorize commits, pushes, or PRs.
