// Package postgres provides the opt-in PostgreSQL driver adapter for the database module,
// the standard cloud backend alongside the local sqlite adapter.
//
// Importing this package has no side effects.
// Call Register with an explicit database.DialectRegistry before selecting it, and use
// MigrateDriver to wire golang-migrate against the same connection.
package postgres
