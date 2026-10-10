// Package migration runs schema migrations against a database.
//
// Config carries the database, filesystem source, path, context-bound DriverFunc and metadata Table. Operations execute synchronously without prefetch goroutines, bound each SQL file, validate the whole source and the recorded version before changing anything, and preserve dirty state on failure. Apply migrates to an exact target; Up applies everything. Ready checks an expected schema version in addition to connectivity and reports an unmigrated, dirty or mismatched schema as not ready. Version and Ready are read-only, take no migration lock and need only SELECT on the version table, so a runtime role without DDL rights can call them.
//
// Independently owned migration sets that share a database each name their own metadata Table. Session pins one connection and holds the backend migration lock so a coordinator can plan and apply several Sets, with backend schema work in between, under one lock on a pool limited to one connection.
package migration
