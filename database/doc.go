// Package database provides a GORM-based database component with connection pooling, health checks,
// transactions, and migration support.
//
// # Architecture
//
// The database module follows gokit's component pattern with a driver-agnostic design.
// Users provide the database backend as a Dialect (postgres, sqlite, etc.) via WithDialect()
// or an explicit DialectRegistry, keeping runtime backend selection adapter-driven.
//
// # Quick Start
//
// Bootstrap the database component in your application:
//
//	import (
//	    "github.com/kbukum/gokit/bootstrap"
//	    "github.com/kbukum/gokit/database"
//	    "github.com/kbukum/gokit/database/postgres"
//	)
//
//	func main() {
//	    app := bootstrap.New()
//	    cfg := database.Config{Enabled: true, DSN: "host=localhost user=myuser dbname=mydb"}
//	    app.Register(database.NewComponent(cfg, log).
//	        WithDialect(postgres.Dialect()))
//	    app.Start(context.Background())
//	}
//
// # Subpackages
//
//   - errors: Database error utilities and translation to AppError
//   - types: Common database types like BaseModel
//   - migration: File-based database migrations using golang-migrate
//   - query: Advanced query builders and helpers
//   - sqlite: Opt-in SQLite driver adapter
//   - postgres: Opt-in PostgreSQL driver adapter
//   - testutil: Testing utilities for database-dependent tests
//
// # Optional Component
//
// The database component respects the Enabled flag in configuration. When disabled,
// Start() returns immediately without initializing the connection,
// and Health() reports "disabled" status.
//
//	cfg := database.Config{Enabled: false}  // Component will be disabled
//
// See component.go for full lifecycle documentation.
package database
