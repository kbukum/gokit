# Architecture Decision Records (ADR)

ADRs document **significant** architectural choices: tradeoffs considered, why we picked one option,
and the consequences. They are immutable — superseded ADRs are kept and linked from the replacement.

Closes F-065 sub-finding: `docs/` had no `docs/adr/`.

## When to write an ADR

- Adding or replacing a foundation tier package
- Introducing a transport-layer dependency
- Changing the lifecycle / boot model
- Anything that locks downstream callers into a pattern that is costly to reverse

## Format

Each ADR is a Markdown file named `NNNN-short-kebab-title.md` where NNNN is the next zero-padded sequence number.
Use [`0000-template.md`](0000-template.md) as the starting point.

## Index

| #     | Title                                              | Status   |
|-------|----------------------------------------------------|----------|
| 0001  | [Three-tier package layering](0001-three-tier-layering.md) | Accepted |
