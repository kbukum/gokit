# database/sqlite

SQLite driver adapter for `github.com/kbukum/gokit/database`.

The core `database` module owns the database component, repository helpers, and `DialectRegistry`. This adapter owns the SQLite driver dependency and registers itself only when the application explicitly calls `Register`.

## Usage

```go
package main

import (
    "github.com/kbukum/gokit/database"
    "github.com/kbukum/gokit/database/sqlite"
)

func configure() (*database.DialectRegistry, error) {
    registry := database.NewDialectRegistry()
    if err := sqlite.Register(registry); err != nil {
        return nil, err
    }
    return registry, nil
}
```

Importing this package has no side effects. Applications own the registry and choose the driver through configuration.

## Connection policy

Run **one process per SQLite file**. The adapter enforces WAL, foreign keys and a 5-second busy timeout through connection options, so replacement connections receive the same settings. Writes use one connection; `database.DB.ReadOnly(ctx)` uses a separate pool of four connections with `query_only` enabled. The database closes both pools. In-memory databases use one shared connection without lifetime or idle expiry, and do not create a separate reader. Closing that connection destroys the database.

Use the managed writer for all writes and migration sessions. Do not open another writable pool against the file. Context cancellation stops waiting for the pool; SQLite lock waits can still take up to the 5-second busy timeout inside the C driver. Avoid long transactions and external writers.

Concurrency coverage uses eight reader callers and four writer callers, 100 operations each, within a 30-second budget. Pool limits remain four readers and one writer.

## Migrations

Use `sqlite.MigrateDriver()` in `migration.Config`, then call `Up(ctx)` and `Ready(ctx, expectedVersion)`. The factory is a real context-bound SQL driver, not a test double. It owns its borrowed connection, not the application pool. A dirty or unexpected schema version must fail application readiness.
