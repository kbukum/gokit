# database

GORM-backed database contracts, component lifecycle, transaction helpers, repository helpers, migrations, query builder, and tenant utilities.

Core does not select a backend by default. Applications register or inject the dialect they need as a `Dialect`; adapter packages must not use import-time registration.

A backend is a `Dialect` — it names itself and opens a GORM connection for a DSN. `Config.DSN` is an opaque connection string whose format is defined by that dialect. Core is driver-agnostic and never assembles a DSN itself: when you supply structured `Config.Params` instead of a DSN, the selected dialect builds the DSN (it must implement `StructuredDialect`, as `database/postgres` does). SQLite has no structured form, so callers set `DSN` (a path or `:memory:`) directly.

## Explicit dialect

```go
import "github.com/kbukum/gokit/database/postgres"

cfg := database.Config{
    Enabled:     true,
    DSN:         "host=localhost user=app dbname=mydb sslmode=disable",
    AutoMigrate: true,
}

comp := database.NewComponent(cfg, log).
    WithDialect(postgres.Dialect()).
    WithAutoMigrate(&User{})
```

## Structured params

Let the dialect build the DSN from typed fields instead of a hand-written string. Backend-specific knobs (`sslmode`, `tls`, …) go in `Options`, so the common shape stays shared across drivers:

```go
cfg := database.Config{
    Enabled: true,
    Params: database.ConnParams{
        Host:     "localhost",
        User:     "app",
        Database: "mydb",
        Options:  map[string]string{"sslmode": "disable"},
    },
}

comp := database.NewComponent(cfg, log).WithDialect(postgres.Dialect())
```

## Registry-driven selection

Register several dialects in one registry and pick the backend by name through configuration:

```go
import (
    "github.com/kbukum/gokit/database/postgres"
    "github.com/kbukum/gokit/database/sqlite"
)

dialects := database.NewDialectRegistry()
if err := sqlite.Register(dialects); err != nil {
    return err
}
if err := postgres.Register(dialects); err != nil {
    return err
}

comp := database.NewComponent(cfg, log).
    WithDialectFromRegistry(dialects, postgres.Name)
```

## SQLite adapter

`database/sqlite` is the single-process local backend. File databases use WAL, a 5-second busy timeout on every connection, one write connection and four read-only connections:

```go
import "github.com/kbukum/gokit/database/sqlite"

dialects := database.NewDialectRegistry()
if err := sqlite.Register(dialects); err != nil {
    return err
}

comp := database.NewComponent(database.Config{
    Enabled: true,
    DSN:     ":memory:",
}, log).WithDialectFromRegistry(dialects, sqlite.Name)
```

## PostgreSQL adapter

`database/postgres` is the nested adapter module for the standard cloud backend. It mirrors the SQLite adapter — explicit `Register`, no import-time side effects — and additionally exposes `MigrateDriver()` so the driver-agnostic `migration` package works against Postgres:

```go
import "github.com/kbukum/gokit/database/postgres"

dialects := database.NewDialectRegistry()
if err := postgres.Register(dialects); err != nil {
    return err
}

comp := database.NewComponent(database.Config{
    Enabled: true,
    DSN:     "host=localhost user=app dbname=app sslmode=disable",
}, log).WithDialectFromRegistry(dialects, postgres.Name)
```

Register both adapters and let configuration pick the backend name — `sqlite` locally, `postgres` in the cloud — through the same registry.

## Drivers

| Name | Adapter module | GORM driver | golang-migrate driver | Typical use |
|---|---|---|---|---|
| `sqlite` | `database/sqlite` | `gorm.io/driver/sqlite` | `sqlite.MigrateDriver()` | local / tests |
| `postgres` | `database/postgres` | `gorm.io/driver/postgres` | `postgres.MigrateDriver()` | cloud |

Cross-kit parity: mirrors rskit's database driver contrib, keeping backend selection registry-driven and adapter-owned in both kits.

## Bounded lists

`repository.List` and `query.ApplyToGorm` always paginate. Zero and negative page sizes use the configured default (20 normally); oversized values clamp to the configured maximum, never above 100. Page numbers below one become one. There is no unbounded list mode. Empty offset pages contain `data: []`, and `totalPages` is at least one. Facets return an error if they exceed their distinct-value limit, rather than silently truncating.

Use `repository.ListCursor` or `query.ApplyCursorToGorm` for frequently changing lists. The server supplies `CursorConfig`: an immutable, non-null sort field, the model's single primary key as a tie-breaker, and a scope identifying the resource, tenant and hidden base-query restrictions. Cursors bind the filter, search, scope and ordering; changing them requires a fresh traversal. Cursor paging is forward-only and omits `nextCursor` at the end. It is not a snapshot: inserts behind an already-consumed position are not replayed.

Ordering supports strings, signed and unsigned integers (including named scalar types), `time.Time` and `uuid.UUID`. Pointer fields, nullable SQL wrappers, custom SQL valuers/scanners and GORM serializers are rejected before querying, even when a column is declared `NOT NULL`.

```go
page, err := query.ApplyCursorToGorm[Run](ctx, db.ReadOnly(ctx).Model(&Run{}),
    query.CursorParams{PageSize: 20, Cursor: cursor},
    query.CursorConfig{Scope: workspaceID + "/runs", OrderBy: "created_at", UniqueBy: "id", Descending: true})
```

A cursor is an untrusted position, not an authorization credential. Apply access restrictions on every query; a cursor never grants access. Shared response fixtures live in `query/testdata/lists.json`.

## Connections and transactions

`WithContext(ctx)` selects the primary pool; `ReadOnly(ctx)` selects the reader when the adapter provides one. SQLite writes and transactions use the single writer. In-memory SQLite uses a single shared connection, so its schema survives between operations. `Close` closes both owned pools.

Use `WithTransaction(ctx, fn)` for short transactions. It rolls back on errors, cancellation and panics, and commits only while the context remains active. `WithReadOnlyTransaction` always rolls back and uses backend read-only enforcement where supported. `AutoMigrate` also requires a context.

Both adapters preserve GORM's savepoint support for nested transactions and constraint-error translation when `gorm.Config.TranslateError` is enabled.

## Migrations and readiness

Both adapters provide `MigrateDriver()`. `migration.Config` keeps the golang-migrate file/source and schema-version conventions, but runs synchronously with caller cancellation and a default two-minute budget. Each SQL file is limited to 1 MiB and runs in a transaction; files must not contain transaction-control statements. No migration goroutine or prefetch queue is started. A failure leaves the version dirty.

`Reset(ctx)` is destructive: it drops tables transactionally, then reapplies migrations. SQLite defers foreign-key checks only inside the drop transaction; PostgreSQL cascades dependent objects, including partitions and constraints. The drop phase rolls back on failure; reapplying migrations is a separate phase and can leave a dirty version if it fails.

Query logs contain duration, row counts, and error types, never interpolated SQL or raw driver error messages. Detailed errors remain available to the caller through their preserved cause.

Call `Up(ctx)` during startup, then `Ready(ctx, expectedVersion)` from the application's readiness check as well as checking database connectivity. A successful ping alone says nothing about schema readiness. Migration sessions borrow a connection and release it without closing the application pool. PostgreSQL serializes concurrent migrators with an advisory lock; SQLite requires one process and one managed writer pool per file.

## Expiry cleanup and metrics

`cleanup.DeleteExpired[T](ctx, db.GormDB, cfg)` deletes one bounded batch, not an entire backlog. Schedule it with the existing `worker.TickerWorker` or `worker.Scheduler`; pass their cancellation context and report errors. Use an index on the expiry field and primary key. The default batch is 100, the maximum is 1,000, and each batch has a 30-second budget. An injected `util.Clock` controls the expiry cutoff.

`metrics.Register(db, meter, "app")` observes open, in-use and idle connections, wait count/time and slow-query count. Supply a stable server-configured name, unique for each database registered with that meter, limited to 128 bytes without surrounding whitespace. Every metric includes the `database` identity; pool metrics also distinguish the fixed primary/reader pools. Never use DSNs, query text, tenants or row IDs as identities. Registration starts no polling goroutine. Unregister the callback before shutting down telemetry.

## Design constraints

- Component startup requires an explicit dialect or registry selection.
- `DialectRegistry` stores dialects without package-level global state.
- Runtime code stays driver-agnostic; backend adapters register with an application-owned registry.
- GORM provides the repository/query substrate for the Go implementation.
