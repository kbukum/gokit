# database

GORM wrapper with connection pooling, migrations, query builder, and component lifecycle management.

## Install

```bash
go get github.com/kbukum/gokit/database@latest
```

## Quick Start

### Production (PostgreSQL, MySQL, etc.)

```go
import (
    "github.com/kbukum/gokit/database"
    "github.com/kbukum/gokit/logger"
    "gorm.io/driver/postgres"  // Import your chosen driver
)

cfg := database.Config{
    Enabled:     true,
    DSN:         "host=localhost user=app dbname=mydb sslmode=disable",
    AutoMigrate: true,
}
cfg.ApplyDefaults()

log := logger.New()
comp := database.NewComponent(cfg, log).
    WithDriver(postgres.Open).  // Specify driver function
    WithAutoMigrate(&User{})

// Start as a managed component
comp.Start(ctx)
defer comp.Stop(ctx)

// Use the DB wrapper
db := comp.DB()
db.WithContext(ctx).Find(&users)
db.Transaction(func(tx *gorm.DB) error {
    return tx.Create(&User{Name: "alice"}).Error
})
```

### Optional Components (Disable via Config)

```go
// Database is optional - set Enabled: false to skip initialization
cfg := database.Config{
    Enabled: false,  // Component skips Start(), returns healthy status
    DSN:     "...",
}

comp := database.NewComponent(cfg, log)
comp.Start(ctx)  // No-op, logs "Database component is disabled"
comp.Health(ctx) // Returns StatusHealthy with "disabled" message
```

### Tests/Development (SQLite default)

```go
// No driver import needed - SQLite is the default
comp := database.NewComponent(database.Config{
    DSN: ":memory:",  // In-memory SQLite
}, log)
```

## Driver Pattern

**Clean, flexible driver injection with no forced dependencies.**

### How It Works

```go
type DriverFunc func(dsn string) gorm.Dialector

// Pass the driver function (not the result of calling it)
db := database.NewComponent(cfg, log).WithDriver(postgres.Open)

// Component calls it during Start() to control lifecycle
dialector := driverFunc(cfg.DSN)
```

### Supported Drivers

All standard GORM drivers work:

```go
import "gorm.io/driver/postgres"
WithDriver(postgres.Open)

import "gorm.io/driver/mysql"
WithDriver(mysql.Open)

import "gorm.io/driver/sqlite"
WithDriver(sqlite.Open)  // or omit for default

import "gorm.io/driver/sqlserver"
WithDriver(sqlserver.Open)
```

### Key Benefits

- **No forced dependencies** - Only SQLite imported by default
- **User controls driver** - Import what you need
- **Lifecycle control** - Driver called during Start(), enabling retry logic
- **Clean defaults** - SQLite works out of the box for tests

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Component` | Managed lifecycle wrapper — `Start`, `Stop`, `Health` |
| `Config` | DSN, pool sizes, slow-query threshold, auto-migrate flag |
| `DB` | GORM wrapper — `WithContext`, `Transaction`, `CheckHealth`, `AutoMigrate` |
| `BaseModel` | UUID primary key, timestamps, soft-delete |
| `NewComponent(cfg, log)` | Create a managed database component |
| `New(cfg, log)` | Create a standalone `*DB` without lifecycle |
| `IsNotFoundError(err)` | Check for record-not-found |
| `IsDuplicateError(err)` | Check for unique constraint violation |

### Sub-packages

| Package | Description |
|---|---|
| `database/migration` | Driver-agnostic migrations using `DriverFunc`. Import your database's migrate driver (e.g., `migrate/v4/database/postgres`). Supports `MigrateUp`, `MigrateDown`, `MigrateReset`, and `MigrationRunner` for programmatic migrations. |
| `database/query` | `ParseFromRequest` → `Params`; `ApplyToGorm[T]` → `*Result[T]` with pagination, filtering, sorting, facets, and includes |

## Migration Usage

Migrations are now driver-agnostic. Import the appropriate golang-migrate driver for your database:

```go
import (
    "embed"
    "github.com/kbukum/gokit/database/migration"
    migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PostgreSQL
err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations",
    func(sqlDB *sql.DB) (database.Driver, error) {
        return migratepg.WithInstance(sqlDB, &migratepg.Config{})
    },
)

// MySQL
import migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations",
    func(sqlDB *sql.DB) (database.Driver, error) {
        return migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
    },
)

// SQLite
import migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations",
    func(sqlDB *sql.DB) (database.Driver, error) {
        return migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
    },
)
```

---

[← Back to main gokit README](../README.md)
