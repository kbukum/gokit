---
name: fix-reviews
description: "gokit: Evaluate PR review comments and fix valid patterns across the affected change; only when asked."
user-invocable: true
---

# Fix review findings

Only act when requested. Evaluate comments against the [baseline](../../copilot-instructions.md), not the reviewer's authority.

1. Identify the PR and collect reviews, inline comments, and paginated GraphQL threads, including resolution state and node IDs.
2. Accept valid findings; reject conflicting or incorrect advice with a brief reason.
3. Identify the underlying pattern and fix every instance in the PR change set. Include tightly coupled pre-existing defects and affected callers; do not silently widen into unrelated work.
4. Test behavioral fixes first, including failure paths. Prefer one owning fix over repeated patches.
5. Run scoped [validation](../validate/SKILL.md). Prose-only changes need documentation checks.
6. With explicit Git authorization, [commit](../commit/SKILL.md) and push the fixes. Resolve only threads whose validated fixes are on the remote.

```bash
gh pr view <n> --json reviews,comments
gh api repos/{owner}/{repo}/pulls/<n>/comments --paginate
git diff origin/main...HEAD
```

Resolve an addressed thread by node ID:

```bash
gh api graphql -f query='
  mutation($id:ID!){ resolveReviewThread(input:{threadId:$id}){ thread{ isResolved } } }' \
  -F id=<threadId>
```

Fixing reviews does not itself authorize commits, amendments, or pushes. Follow the commit skill's message rules; do not post replies on the maintainer's behalf. Leave rejected or decision-blocked threads unresolved and report why.
