// Package migration runs schema migrations against a database.
//
// [MigrateUp], [MigrateDown], [MigrateSteps], [MigrateVersion],
// and [MigrateReset] drive a migration source through a [DriverFunc],
// giving composition roots explicit,
// ordered control over schema evolution instead of implicit auto-migration.
package migration
