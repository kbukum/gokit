# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in gokit, please report it responsibly.

### What to Expect

- **Acknowledgment** within 48 hours of your report
- **Status update** within 5 business days with an assessment
- **Fix timeline** communicated once the issue is confirmed
- **Credit** in the release notes (unless you prefer to remain anonymous)

### Disclosure Policy

- We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure).
- Please allow us reasonable time to address the issue before any public disclosure.
- We will work with you to understand and resolve the issue promptly.

## Security Best Practices for Users

When using gokit in production:

- Keep dependencies up to date (`make update && make tidy`)
- Use the `encryption` package for sensitive data at rest
- Configure TLS via the `security` package for all network communication
- Never commit secrets — use environment variables or secret managers
- Review `gosec` findings regularly (`make lint` includes `gosec`)
