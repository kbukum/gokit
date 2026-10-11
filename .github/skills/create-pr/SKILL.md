---
name: create-pr
description: "gokit: Open a draft pull request using the repository template; only when explicitly asked."
user-invocable: true
---

# Open a draft PR

Only when explicitly requested:

1. Confirm the branch is committed and pushed, the worktree is clean, and the base is `main`. Push only with authorization.
2. Read the actual branch diff and close affected contracts. The PR must stand alone, without plan or batch context.
3. Fill [the PR template](../../PULL_REQUEST_TEMPLATE.md) honestly.
4. Create a draft and report its URL.

```bash
gh pr create --draft --base main --title "<conventional-title>" --body-file <path>
```

## Description

- Use a short capability-focused title and explain what changed and why.
- Group changes by concern; avoid file inventories, code recipes, and execution history.
- State affected modules, public-contract changes, and necessary pre-stable breaking changes.
- Check only verified testing and checklist items. Distinguish unit, adapter, process, and integration evidence.
- For public abstractions, confirm [sibling parity](../parity/SKILL.md) or identify the outstanding gap. Cross-repository references use full URLs.

Apply the [baseline](../../copilot-instructions.md). Review and fix in-scope defects before publication; green tooling alone is insufficient. Never request reviewers or trigger a review implicitly.
