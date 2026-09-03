# database/postgres

PostgreSQL driver adapter for `github.com/kbukum/gokit/database`.

The core `database` module owns the database component, repository helpers, `DialectRegistry`, and the driver-agnostic `migration` package. This adapter owns the PostgreSQL driver dependency (`gorm.io/driver/postgres`) and its golang-migrate driver, and registers itself only when the application explicitly calls `Register` — there is no import-time registration.

Postgres is the standard cloud backend; `database/sqlite` is the local/dev counterpart. Both are selected through the same registry, so an application chooses its backend by configuration.

## Usage

```go
package main

import (
    "github.com/kbukum/gokit/database"
    "github.com/kbukum/gokit/database/postgres"
    "github.com/kbukum/gokit/database/sqlite"
)

// configure registers both adapters and lets configuration pick the backend:
// sqlite for local development, postgres for cloud.
func configure() (*database.DialectRegistry, error) {
    registry := database.NewDialectRegistry()
    if err := sqlite.Register(registry); err != nil {
        return nil, err
    }
    if err := postgres.Register(registry); err != nil {
        return nil, err
    }
    return registry, nil
}
```

Then start the component from the registry using the configured backend name:

```go
comp := database.NewComponent(cfg, log).
    WithDialectFromRegistry(registry, postgres.Name)
```

## Building a DSN

Core `database.Config` is driver-agnostic: it takes either an opaque `DSN` string or structured
`database.ConnParams`, and does not know PostgreSQL's connection-string shape. The PostgreSQL
dialect implements `database.StructuredDialect`, so when you set `Config.Params` (and leave `DSN`
empty) the component builds the DSN for you:

```go
cfg := database.Config{
    Enabled: true,
    Params: database.ConnParams{
        Host:     "db.example",
        User:     "app",
        Password: os.Getenv("DB_PASSWORD"),
        Database: "app",
    },
}

comp := database.NewComponent(cfg, log).WithDialect(postgres.Dialect())
```

A zero `Port` defaults to `5432` and an absent `Options["sslmode"]` to `verify-full`, so
connections are encrypted and the server certificate is verified by default. Set
`Options["sslmode"] = "disable"` explicitly to opt out on a trusted local network. Every field —
user, password, host, path, and option value — is URL-encoded (IPv6 hosts are bracketed), and the
dialect emits a URL-form DSN that GORM's PostgreSQL driver accepts.

## Migrations

The core `migration` package is driver-agnostic and needs a backend-specific golang-migrate driver. `MigrateDriver` supplies the PostgreSQL one, keyed to the same connection GORM manages:

```go
cfg := migration.Config{
    DB:     gormDB,
    FS:     migrationsFS,
    Path:   "migrations",
    Driver: postgres.MigrateDriver(),
}
if err := cfg.Up(); err != nil {
    return err
}
```

## Testing

Integration tests live in `*_integration_test.go` behind the `integration` build tag and provision an ephemeral PostgreSQL server with [`testcontainers-go`](https://golang.testcontainers.org/), so they never depend on a local Postgres and skip when no Docker daemon is reachable:

```bash
go test -tags=integration ./database/postgres/...
```

Importing this package has no side effects. Applications own the registry and choose the driver through configuration.
