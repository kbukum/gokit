// Package database provides a GORM-based database component with connection pooling,
// health checks, transactions, and migration support.
//
// # Architecture
//
// The database module follows gokit's component pattern with a driver-agnostic design.
// Users provide the database driver (postgres, mysql, sqlite, etc.) via WithDriver(),
// keeping the module lightweight and flexible.
//
// # Quick Start
//
// Bootstrap the database component in your application:
//
//	import (
//	    "github.com/kbukum/gokit/bootstrap"
//	    "github.com/kbukum/gokit/database"
//	    "github.com/kbukum/gokit/database/core"
//	    "gorm.io/driver/postgres"
//	)
//
//	func main() {
//	    app := bootstrap.New()
//	    cfg := core.Config{Enabled: true, DSN: "host=localhost user=myuser password=mypass dbname=mydb"}
//	    app.Register(database.NewComponent(cfg, log).
//	        WithDriver(func(dsn string) gorm.Dialector {
//	            return postgres.Open(dsn)
//	        }))
//	    app.Start(context.Background())
//	}
//
// # Subpackages
//
//   - core: Core database wrapper, connection pooling, and transaction helpers
//   - errors: Database error utilities and translation to AppError
//   - types: Common database types like BaseModel
//   - migration: File-based database migrations using golang-migrate
//   - query: Advanced query builders and helpers
//   - testutil: Testing utilities for database-dependent tests
//
// # Optional Component
//
// The database component respects the Enabled flag in configuration.
// When disabled, Start() returns immediately without initializing the connection,
// and Health() reports "disabled" status.
//
//	cfg := core.Config{Enabled: false}  // Component will be disabled
//
// See component.go for full lifecycle documentation.
package database
