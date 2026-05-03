package sqlite

import (
	"github.com/kbukum/gokit/database"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const Name = "sqlite"

// Register registers the SQLite GORM driver in an explicit database registry.
func Register(reg *database.DriverRegistry) error {
	return reg.Register(Name, gormsqlite.Open)
}

// Open returns a SQLite GORM dialector.
func Open(dsn string) database.DriverFunc {
	return func(_ string) gorm.Dialector {
		return gormsqlite.Open(dsn)
	}
}
