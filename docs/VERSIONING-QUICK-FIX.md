# Versioning Quick Fix

Use this procedure when a consumer resolves a gokit module to a zero-date pseudo-version.

## Cause

The module's version tag is missing from the remote repository, or the consumer has not requested a published version.

## Fix

From a clean `main` checkout, preview the tags first:

```bash
toven release plan
```

Configure signing and cut the complete module tag family:

```bash
git config tag.gpgsign true
git config user.signingkey <KEY-ID>
toven release publish --set-version 0.3.0-alpha.1 --yes
```

Toven discovers all `go.mod` files automatically. It creates the root tag (`v0.3.0-alpha.1`) and path-scoped tags such as `auth/v0.3.0-alpha.1` and `database/sqlite/v0.3.0-alpha.1`.

Consumers can then request the published version:

```bash
go get github.com/kbukum/gokit@v0.2.0
go get github.com/kbukum/gokit/auth@v0.2.0
go get github.com/kbukum/gokit/database/sqlite@v0.2.0
go mod tidy
```

For local development, use `replace` directives instead of publishing temporary tags. See [`VERSIONING.md`](VERSIONING.md) for the complete policy.
