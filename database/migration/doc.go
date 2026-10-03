// Package migration runs schema migrations against a database.
//
// Config carries the database, filesystem source, path and context-bound DriverFunc. Operations execute synchronously without prefetch goroutines, bound each SQL file, and preserve dirty state on failure. Ready checks an expected schema version in addition to connectivity.
package migration
