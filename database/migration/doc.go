// Package migration runs schema migrations against a database.
//
// [Config] carries the GORM database, embedded source, path, and [DriverFunc]; its Up, Down, Steps,
// Version, and Reset methods drive a migration source through the driver,
// giving composition roots explicit,
// ordered control over schema evolution instead of implicit auto-migration.
package migration
