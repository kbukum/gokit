# Resolve a missing module version

A zero-date pseudo-version usually means the module's path-scoped tag is missing or the consumer did not select a published version.

1. Inspect remote tags and the Go proxy for the affected module.
2. Request an existing published version, or use a local `replace` for unreleased development.
3. If publication is needed, follow the complete [release procedure](RELEASING.md). Do not cut ad-hoc tags or reuse a cached version.

```bash
go get github.com/kbukum/gokit/auth@v0.2.0
go mod tidy
```

See [Versioning](VERSIONING.md) for root and nested tag conventions.
