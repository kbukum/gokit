# database

GORM wrapper with connection pooling, migrations, query builder, and component lifecycle management.

## Install

```bash
go get github.com/skillsenselab/gokit/database@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/database"
    "github.com/skillsenselab/gokit/logger"
)

log := logger.New()
comp := database.NewComponent(database.Config{
    Enabled:     true,
    DSN:         "host=localhost user=app dbname=mydb sslmode=disable",
    AutoMigrate: true,
}, log).WithAutoMigrate(&User{})

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
| `database/migration` | `MigrateUp`, `MigrateDown`, `MigrateReset` via embedded FS; `MigrationRunner` for programmatic migrations |
| `database/query` | `ParseFromRequest` → `Params`; `ApplyToGorm[T]` → `*Result[T]` with pagination, filtering, sorting, facets, and includes |

---

[← Back to main gokit README](../README.md)
