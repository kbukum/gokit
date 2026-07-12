# database

GORM-backed database contracts, component lifecycle, transaction helpers, repository
helpers, migrations, query builder, and tenant utilities.

Core does not select a backend driver by default. Applications register or inject the
driver they need; adapter packages must not use import-time registration.

## Explicit driver

```go
import "gorm.io/driver/postgres"

cfg := database.Config{
    Enabled:     true,
    DSN:         "host=localhost user=app dbname=mydb sslmode=disable",
    AutoMigrate: true,
}

comp := database.NewComponent(cfg, log).
    WithDriver(postgres.Open).
    WithAutoMigrate(&User{})
```

## Registry-driven selection

```go
import "gorm.io/driver/postgres"

drivers := database.NewDriverRegistry()
if err := drivers.Register("postgres", postgres.Open); err != nil {
    return err
}

comp := database.NewComponent(cfg, log).
    WithDriverFromRegistry(drivers, "postgres")
```

## SQLite adapter

`database/sqlite` is a nested adapter module for tests/local development:

```go
import "github.com/kbukum/gokit/database/sqlite"

drivers := database.NewDriverRegistry()
if err := sqlite.Register(drivers); err != nil {
    return err
}

comp := database.NewComponent(database.Config{
    Enabled: true,
    DSN:     ":memory:",
}, log).WithDriverFromRegistry(drivers, sqlite.Name)
```

## Design constraints

- Component startup requires an explicit driver or registry selection.
- `DriverRegistry` stores backend factories without package-level global state.
- Runtime code stays driver-agnostic; backend adapters register with an application-owned registry.
- GORM provides the repository/query substrate for the Go implementation.
