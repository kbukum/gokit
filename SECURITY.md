# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in gokit, please report it
**privately** using one of the following channels:

1. **Preferred**: [GitHub Security Advisories](https://github.com/kbukum/gokit/security/advisories/new)
   — opens a private disclosure thread visible only to maintainers.
2. **Alternative**: Email the maintainers (see [MAINTAINERS.md](MAINTAINERS.md))
   with subject prefix `[SECURITY]`.

Do **not** open a public GitHub issue for security reports.

### What to Include

- A clear description of the issue and its potential impact.
- Steps to reproduce, including a minimal proof-of-concept if possible.
- The affected version(s) and platform(s).
- Any suggested mitigations or fixes.

### What to Expect

- **Acknowledgment** within 48 hours of your report.
- **Status update** within 5 business days with an assessment.
- **Fix timeline** communicated once the issue is confirmed.
- **CVE assignment** for confirmed vulnerabilities affecting released
  versions, requested via GitHub Security Advisories.
- **Credit** in the release notes and the advisory (unless you prefer to
  remain anonymous).

### Disclosure Policy

- We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure).
- Please allow a reasonable embargo period (typically 90 days) before any
  public disclosure, extendable by mutual agreement when a fix requires
  coordination across downstream consumers.
- Once a fix is released, the advisory is published and CVE details are
  made public.

## Security Best Practices for Users

When using gokit in production:

- Keep dependencies up to date (`make update && make tidy`).
- Use the `encryption` package for sensitive data at rest.
- Configure TLS via the `security` package for all network communication
  — never set `InsecureSkipVerify: true` in production.
- Never commit secrets — use environment variables or secret managers.
- Review `gosec` and `govulncheck` findings regularly (`make lint` includes
  `gosec`; CI runs `govulncheck` per module on every push).
- For HTTP authentication, prefer the secure-by-default `Auth`/`OptionalAuth`
  middleware. Avoid enabling `WithQueryTokenParam` unless absolutely
  necessary, and always pair it with `WithQueryTokenAllowedPaths` and
  `WithQueryTokenWarningLogger`.

## Supply Chain

- All GitHub Actions used in CI are pinned to commit SHAs (see
  `.github/workflows/`).
- Dependency updates are automated via Dependabot
  (`.github/dependabot.yml`).
- Module Go versions are kept consistent across all sub-modules; the CI
  `version-check` job enforces this invariant.
- Every dependency's license is checked against a permissive allow-list on
  each push (`scripts/check-licenses.sh`); copyleft and unknown licenses fail
  CI. New direct dependencies are justified and audited in
  [`docs/dependencies.md`](docs/dependencies.md).

