# gokit development skills

[Agent Skills](https://docs.github.com/copilot/concepts/agents/about-agent-skills) for developing
**gokit itself** — loaded on demand by GitHub Copilot (CLI, coding agent, code review, IDEs) when
a task matches a skill's description. These are **project skills** for contributors; they do not
affect anyone who consumes gokit as a library.

Each skill is a folder with a `SKILL.md` (YAML frontmatter + workflow) and optional bundled
reference files loaded only when the skill activates (progressive disclosure). They encode gokit's
permanent engineering baseline (see [`../copilot-instructions.md`](../copilot-instructions.md)) and
drive tasks through [`toven`](../../toven.toml), the repo's argv-first task planner.

## Skills

| Skill | Use when |
|---|---|
| [`create-branch`](create-branch/SKILL.md) | Cut a branch off an up-to-date main, named by the high-level change (no batch/plan/internal detail). |
| [`create-plan`](create-plan/SKILL.md) | Turn a non-trivial change into a reviewable plan under `tmp/` — README + numbered step files, bound to the baseline. |
| [`apply-plan`](apply-plan/SKILL.md) | Execute a `tmp/` plan from its first unfinished step onward, validating after each; resumable. |
| [`apply-step`](apply-step/SKILL.md) | Apply one plan step in context (README + prior steps), test-first against the baseline, then mark it done. |
| [`commit`](commit/SKILL.md) | Commit staged work with one compact, developer-friendly message — no co-author trailer or plan/batch/tool narration. |
| [`create-pr`](create-pr/SKILL.md) | Open a reviewer-friendly PR — high-level summary, honest template sections, bound to the baseline. |
| [`evaluate-reviews`](evaluate-reviews/SKILL.md) | Act on PR review comments by pattern — fix every instance across the change set, then commit and resolve the threads. |
| [`validate`](validate/SKILL.md) | Build/test/lint/tidy/vuln a change through toven, scoped to the affected modules. |
| [`review`](review/SKILL.md) | Run the eight-pass engineering-baseline review over a diff, module, or the tree. |
| [`new-module`](new-module/SKILL.md) | Scaffold a new package/module — placement, go.mod, doc.go, domains.toml, go.work. |
| [`new-backend`](new-backend/SKILL.md) | Add a storage/vectorstore/messaging/cache/llm adapter as a typed-registry contrib sub-module. |
| [`parity`](parity/SKILL.md) | Mirror an rskit capability by capability and keep the parity matrix accurate. |
| [`release`](release/SKILL.md) | Cut a release — semver bump, CHANGELOG, full gates, per-module tags. |

## Conventions

- Skills are discoverable in Copilot CLI via `/skills`; project skills live under `.github/skills/`
  (also `.claude/skills` / `.agents/skills` are honored), personal skills under `~/.copilot/skills`.
- Run reviews (`review`) in a **fresh, clean-context agent**, never inline in the session that
  wrote the code.
- Validation is toven-first: prefer `toven <task> --module go:<name>` /
  `toven affected <task> --base origin/main --merge-base` over hand-rolled `go`/`make` commands.
