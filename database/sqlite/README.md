# database/sqlite

SQLite driver adapter for `github.com/kbukum/gokit/database`.

The core `database` module owns the database component, repository helpers, and
`DriverRegistry`. This adapter owns the SQLite driver dependency and registers
itself only when the application explicitly calls `Register`.

## Usage

```go
package main

import (
    "github.com/kbukum/gokit/database"
    "github.com/kbukum/gokit/database/sqlite"
)

func configure() (*database.DriverRegistry, error) {
    registry := database.NewDriverRegistry()
    if err := sqlite.Register(registry); err != nil {
        return nil, err
    }
    return registry, nil
}
```

Importing this package has no side effects. Applications own the registry and
choose the driver through configuration.
