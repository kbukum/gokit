# Governance

This document describes how decisions are made in the gokit project.

## Project Status

gokit is **pre-stable** (`v0.x`).
Backward compatibility is **not** guaranteed between `v0.x` releases;
breaking changes are acceptable when they yield a cleaner long-term design.
See [CHANGELOG.md](CHANGELOG.md) for the full breaking-change history.

## Roles

### Contributors

Anyone who opens an issue or pull request is a contributor.
Contributors are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md)
and the [Contribution Guide](CONTRIBUTING.md).

### Reviewers

Reviewers are contributors who have shown sustained engagement
and are empowered to approve pull requests in specific areas of the code.
Reviewer assignments are recorded in [.github/CODEOWNERS](.github/CODEOWNERS).

### Maintainers

Maintainers have merge rights and are responsible for the long-term direction of the project.
The current list is in [MAINTAINERS.md](MAINTAINERS.md).

## Decision Making

For routine changes (bug fixes, small features), a single maintainer approval is sufficient.
For changes that affect multiple modules or change a public API,
the contributor is encouraged to open a discussion or RFC issue first.

For significant architectural changes (e.g. introducing a new sub-module, removing a public package, changing the release process),
at least two maintainers must approve. If maintainers disagree,
the proposal is deferred until consensus is reached or a clear path forward is documented.

## Release Process

Releases are cut by maintainers using `make tag VERSION=v0.x.y` and pushed via `make tag-push`.
Each release MUST be accompanied by a CHANGELOG entry that follows the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

The single `[Unreleased]` heading invariant is enforced by `scripts/check_changelog_unreleased.sh` in CI.

## Security Issues

Security issues follow the dedicated process in [SECURITY.md](SECURITY.md)
and are not handled via the normal issue tracker.

## Amendments

This document may be amended via pull request.
Amendments require approval from a majority of current maintainers.
