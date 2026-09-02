# database

GORM-backed database contracts, component lifecycle, transaction helpers, repository helpers,
migrations, query builder, and tenant utilities.

Core does not select a backend by default. Applications register or inject the dialect they need
as a `Dialect`; adapter packages must not use import-time registration.

A backend is a `Dialect` — it names itself and opens a GORM connection for a DSN. `Config.DSN` is
an opaque connection string whose format is defined by that dialect. Core is driver-agnostic and
never assembles a DSN itself: when you supply structured `Config.Params` instead of a DSN, the
selected dialect builds the DSN (it must implement `StructuredDialect`, as `database/postgres`
does). SQLite has no structured form, so callers set `DSN` (a path or `:memory:`) directly.

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

Let the dialect build the DSN from typed fields instead of a hand-written string. Backend-specific
knobs (`sslmode`, `tls`, …) go in `Options`, so the common shape stays shared across drivers:

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

`database/sqlite` is a nested adapter module for tests/local development:

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

`database/postgres` is the nested adapter module for the standard cloud backend. It mirrors the
SQLite adapter — explicit `Register`, no import-time side effects — and additionally exposes
`MigrateDriver()` so the driver-agnostic `migration` package works against Postgres:

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

Register both adapters and let configuration pick the backend name — `sqlite` locally, `postgres` in
the cloud — through the same registry.

## Drivers

| Name | Adapter module | GORM driver | golang-migrate driver | Typical use |
|---|---|---|---|---|
| `sqlite` | `database/sqlite` | `gorm.io/driver/sqlite` | user-supplied | local / tests |
| `postgres` | `database/postgres` | `gorm.io/driver/postgres` | `postgres.MigrateDriver()` | cloud |

Cross-kit parity: mirrors rskit's database driver contrib, keeping backend selection registry-driven
and adapter-owned in both kits.

## Design constraints

- Component startup requires an explicit dialect or registry selection.
- `DialectRegistry` stores dialects without package-level global state.
- Runtime code stays driver-agnostic; backend adapters register with an application-owned registry.
- GORM provides the repository/query substrate for the Go implementation.
