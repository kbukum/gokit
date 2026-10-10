// Package repository provides typed repositories over GORM.
//
// [New] constructs a general CRUD repository with transaction support, optimistic locking, and bounded pagination,
// filtering and facet helpers. It applies no access restrictions.
//
// [NewScopedSpec] validates a partitioned model once; [ScopedSpec.Bind] produces a [ScopedRepository] for a pool or
// transaction. Every operation requires a scope value and constrains rows by the configured scope columns: Create
// stamps them, Get/List/Update/Delete filter by them, and missing and out-of-scope rows return the same NotFound.
// Update writes only the configured mutable columns, including zero values, and never inserts. Binding discards
// statement clauses and skips hooks, so a polluted handle cannot widen the scope. Database constraints, such as
// composite foreign keys that include the partition, remain the final guard and keep their own error type.
package repository
