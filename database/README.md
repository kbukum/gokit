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
import (
    "github.com/kbukum/gokit/database"
)

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
| `Component` | Managed lifecycle wrapper — `Start`, `Stop`, `Health`, `Describe` |
| `Config` | DSN, pool sizes, slow-query threshold, auto-migrate flag, enabled flag |
| `DB` | GORM wrapper — `WithContext`, `Transaction`, `PingContext`, `AutoMigrate` |
| `BaseModel` | UUID primary key, timestamps, soft-delete |
| `NewComponent(cfg, log)` | Create a managed database component (defaults to SQLite) |
| `WithDriver(fn)` | Specify database driver (postgres, mysql, etc.) |
| `WithAutoMigrate(models...)` | Register models for auto-migration on startup |
| `New(cfg, log, dialector)` | Create a standalone `*DB` without lifecycle management |
| `NewWithContext(ctx, dialector, cfg, log)` | Create `*DB` with context support (recommended) |
| `IsNotFoundError(err)` | Check for record-not-found |
| `IsDuplicateError(err)` | Check for unique constraint violation |
| `FromDatabase(err, resource)` | Convert database error to AppError |

### Sub-packages

The database module follows a clean, standardized structure:

| Package | Description |
|---|---|
| `database/errors` | Database error utilities and translation to AppError |
| `database/types` | Common database types like BaseModel |
| `database/migration` | Driver-agnostic file-based migrations using `golang-migrate`. Import your database's migrate driver (e.g., `migrate/v4/database/postgres`). Functions: `MigrateUp`, `MigrateDown`, `MigrateSteps`, `MigrateVersion`, `MigrateReset`. |
| `database/query` | HTTP query parsing and advanced filtering: `ParseFromRequest` → `Params`; `ApplyToGorm[T]` → `*Result[T]` with pagination, filtering, sorting, facets, and includes |
| `database/testutil` | In-memory test components with snapshot/restore capabilities for testing |

## Migration Usage

Migrations are now driver-agnostic. Import the appropriate golang-migrate driver for your database:

### PostgreSQL Migrations

```go
import (
    "database/sql"
    "embed"
    "github.com/kbukum/gokit/database/migration"
    "github.com/golang-migrate/migrate/v4/database"
    migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Define driver function
driverFunc := func(sqlDB *sql.DB) (database.Driver, error) {
    return migratepg.WithInstance(sqlDB, &migratepg.Config{})
}

// Run migrations
err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations", driverFunc)
if err != nil {
    log.Fatal("Migration failed", err)
}
```

### MySQL Migrations

```go
import migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"

driverFunc := func(sqlDB *sql.DB) (database.Driver, error) {
    return migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
}

err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations", driverFunc)
```

### SQLite Migrations

```go
import migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"

driverFunc := func(sqlDB *sql.DB) (database.Driver, error) {
    return migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
}

err := migration.MigrateUp(db.GormDB, migrationsFS, "migrations", driverFunc)
```

### Migration Files

Create migration files in your `migrations/` directory:
```
migrations/
  ├── 001_create_users.up.sql
  ├── 001_create_users.down.sql
  ├── 002_add_email_index.up.sql
  └── 002_add_email_index.down.sql
```

### Other Migration Functions

```go
// Roll back all migrations
migration.MigrateDown(db.GormDB, migrationsFS, "migrations", driverFunc)

// Apply/rollback specific number of migrations
migration.MigrateSteps(db.GormDB, migrationsFS, "migrations", 2, driverFunc)  // up 2
migration.MigrateSteps(db.GormDB, migrationsFS, "migrations", -1, driverFunc) // down 1

// Get current version
version, dirty, err := migration.MigrateVersion(db.GormDB, migrationsFS, "migrations", driverFunc)

// Reset database (development only - destroys all data!)
migration.MigrateReset(db.GormDB, migrationsFS, "migrations", driverFunc)
```

---

[← Back to main gokit README](../README.md)
