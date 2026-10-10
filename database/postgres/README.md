# database/postgres

PostgreSQL driver adapter for `github.com/kbukum/gokit/database`.

The core `database` module owns the database component, repository helpers, `DialectRegistry`, and the driver-agnostic `migration` package. This adapter owns the PostgreSQL driver dependency (`gorm.io/driver/postgres`) and context-bound migration backend. It registers itself only when the application explicitly calls `Register`.

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

## Connection input

Core `database.Config` is driver-agnostic: it takes either an opaque `DSN` or structured `database.ConnParams`, never both. The dialect's `Prepare` validates the input and resolves credentials during startup only. Prefer structured params with a mounted password file:

```go
cfg := database.Config{
    Enabled: true,
    Params: database.ConnParams{
        Host:         "db.example",
        User:         "app_runtime",
        PasswordFile: "/run/secrets/database-password",
        Database:     "app",
        Options:      map[string]string{"search_path": "app"},
    },
}

comp := database.NewComponent(cfg, log).WithDialect(postgres.Dialect())
```

A zero `Port` defaults to `5432` and an absent `Options["sslmode"]` to `verify-full` with TLS 1.3; set `"disable"` explicitly only on a trusted local network. An opaque `DSN` must be a credential-free `postgres://` URL; password-bearing URLs and keyword strings are rejected. The password is passed to the driver configuration directly rather than assembled into a URL.

## Migrations

`MigrateDriver` supplies a context-bound PostgreSQL migration session that borrows one connection from GORM's pool. Every `migration.Config` names its metadata table explicitly:

```go
cfg := migration.Config{
    DB:     ownerDB,
    FS:     migrationsFS,
    Path:   "migrations",
    Driver: postgres.MigrateDriver(),
    Table:  migration.Table{Schema: "app", Name: "schema_migrations"},
}
if err := cfg.Apply(ctx, expectedVersion); err != nil {
    return err
}
```

Concurrent migrators are serialized with a database-scoped advisory lock. Lock acquisition and migration statements honor cancellation. Cleanup releases the lock and borrowed connection; an uncertain lock release discards the physical connection rather than returning a locked session to the pool.

## Schema plans and runtime access

Production uses separate roles: a migration role that owns the schema and a runtime role that only reads and writes rows. A `SchemaPlan` names one non-public application schema, the migration `Set`s whose metadata tables live in it, and the runtime role's complete allowlist:

```go
plan := postgres.SchemaPlan{
    Name: "app",
    Sets: []migration.Set{{FS: migrationsFS, Path: "migrations", ExpectedVersion: 7,
        Table: migration.Table{Schema: "app", Name: "schema_migrations"}}},
    RuntimeAccess: postgres.RuntimeAccess{
        Role:      "app_runtime",
        Relations: []postgres.RelationAccess{{Name: "runs", Privileges: []postgres.Privilege{postgres.Select, postgres.Insert}}},
        Sequences: []string{"runs_id_seq"},
    },
}
if err := postgres.ApplySchema(ctx, ownerDB, plan); err != nil { // migration job
    return err
}
comp := database.NewComponent(runtimeCfg, log).
    WithDialect(postgres.Dialect()).
    WithReadiness(postgres.Readiness(plan)) // service
```

`ApplySchema` holds the advisory lock on one pinned connection: it audits the runtime role, plans every set, creates the schema if absent, applies every set to its expected version, reconciles grants to exactly the manifest and verifies the runtime role's effective privileges before the grant transaction commits. It never creates or edits roles and needs only a one-connection pool. There are no default or future-table grants, so a migration that adds a relation must update the manifest. Revoke the default `TEMPORARY` grant from `PUBLIC` on the database; `ApplySchema` rejects a runtime role that can create temporary tables. The runtime role must be a leaf role: membership in any role, whether inherited, `SET`-only or neither, is rejected because it reaches privileges outside the audited matrix. Effective `TRUNCATE`, `REFERENCES`, `TRIGGER`, `MAINTAIN` (PostgreSQL 17+), grant options and callable `SECURITY DEFINER` routines in the schema are also rejected.

`ReadyAccess` and `Readiness` are read-only catalog checks run as the runtime role: expected clean versions, schema `USAGE` without `CREATE`, no database `CREATE` or `TEMPORARY`, the declared relation and sequence rights, and `SELECT`-only version tables. They run the same role and undeclared-object audit as `ApplySchema`, so any state `ApplySchema` rejects also fails readiness. A component with `Readiness` fails `Start` before migration and reports drift from `Health`.

`SetLocal(ctx, tx, "app.partition", value)` sets a transaction-local custom setting for row-level security policies. It rejects handles outside a transaction and binds the value. A setting isolates nothing by itself; policies need `USING` and `WITH CHECK` clauses and a non-owner runtime role.

## Pool and timeout budget

Each new connection receives a 30-second statement timeout and a 10-second idle-in-transaction timeout. These settings are enforced by the adapter rather than inherited from an unbounded server default. Shorter request deadlines still take precedence. The core pool defaults to 25 open and 5 idle connections per instance; include migration sessions in that budget and leave room for administration when sizing `instances × maxOpenConns` against the server limit.

## Testing

Integration tests live behind the `integration` build tag and consume `database/postgres/testutil.Start(ctx)`. This test-only package provisions a separate database/container with a digest-pinned PostgreSQL 17 image. Startup is capped at three minutes, Docker preflight at five seconds, and `fixture.Close(ctx)` gets a fresh thirty-second termination budget even after cancellation. Missing Docker, failed startup, and failed termination are **test failures**, never implicit skips:

```bash
toven --no-cache test --module go:database-postgres -- -tags=integration -race -shuffle=on -count=1 -timeout=10m
```

Importing this package has no side effects. Applications own the registry and choose the driver through configuration.

`fixture.NewDatabase(ctx)` creates an isolated database in which `PUBLIC` has no `CONNECT`, `TEMPORARY` or public-schema `CREATE` right. `db.NewRolePair(ctx, testutil.RolePairConfig{Schema: "app"})` returns fresh owner and runtime login roles with SCRAM-SHA-256 verifiers and no role attributes: the owner can create and migrate the schema, the runtime role can only connect until `ApplySchema` grants it access. `db.Close(ctx)` drops the database and its roles; `fixture.Close` closes any open databases first. Each database and role operation has a 10-second budget. `testutil.WithPasswordFile(t.TempDir(), pair.Runtime)` moves a role's password into a mode-0600 file for `PasswordFile` tests. Fixtures started `WithTLS` give role pairs the same verified TLS settings.

Import `postgres/testutil` only from tests. Register cleanup immediately after successful Start, report any Close error with `t.Error`, and close database clients before the container. The fixture DSN contains generated test credentials; do not log or retain it. Runtime `postgres` and core `database` do not import container provisioning.
