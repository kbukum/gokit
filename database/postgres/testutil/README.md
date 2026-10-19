# PostgreSQL integration fixture

A test-only module that provisions isolated, digest-pinned PostgreSQL containers and hosts the `database/postgres` adapter's integration suite. It is a separate module so the adapter and its consumers never depend on container tooling. Docker is required; missing infrastructure fails rather than skipping.

See the [adapter README](../README.md#testing) for `Start`, `Connect`, `Open`, isolated databases, role pairs and TLS fixtures.

```sh
GOWORK=off go test -tags integration -race -shuffle=on -count=1 ./...
```
