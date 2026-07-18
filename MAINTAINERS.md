# Maintainers

This file lists the people responsible for the gokit project.
Maintainers are responsible for code review, releases, and project direction.

## Core Maintainers

| Name      | GitHub      | Areas             |
|-----------|-------------|-------------------|
| K. Bukum  | @kbukum     | All packages      |

## Bus Factor: 1 — Co-Maintainers Wanted

gokit currently has a **single core maintainer**.
This is a known sustainability risk for a project of this size (34 modules).
We are actively looking for contributors interested in becoming co-maintainers,
particularly in the following areas:

- **Transport/protocol packages**: `grpc`, `connect`, `sse`, `server`
- **Messaging**: `messaging/kafka`, `messaging/managed`
- **Observability**: `observability`, `logger`
- **Security**: `auth`, `auth/oidc`, `encryption`
- **Storage**: `storage/*`, `cache/*`

If you are interested,
please open an issue using the [engineering review template](.github/ISSUE_TEMPLATE/) describing your area of interest
and recent contributions, or start by picking up issues labelled `good-first-issue` / `help-wanted`.

## How Maintainers Are Added

New maintainers are added by the existing core maintainers via a pull request that updates this file.
Candidates are typically long-term contributors who have demonstrated:

- A track record of high-quality contributions across multiple areas of the codebase.
- Familiarity with project conventions, the multi-module layout, and the release process.
- A commitment to responsive code review.

## Responsibilities

Maintainers are expected to:

- Review pull requests within a reasonable timeframe.
- Triage issues and security reports (see [SECURITY.md](SECURITY.md)).
- Cut releases following the process documented in [docs/release-process.md](docs/release-process.md) (when present)
  or the `tag-modules.sh` workflow.
- Uphold the [Code of Conduct](CODE_OF_CONDUCT.md).

## Becoming Inactive / Stepping Down

A maintainer who has been inactive for 6 months may be moved to an "Emeritus" section by the remaining maintainers.
Maintainers are encouraged to step down explicitly by opening a PR to update this file.

## Emeritus Maintainers

_No emeritus maintainers yet._

## Contact

For routine project communication, use GitHub issues or discussions. For security issues,
see [SECURITY.md](SECURITY.md).
