// Package di provides a small, type-keyed dependency injection container.
//
// Dependencies are keyed by their concrete Go type, optionally qualified by a
// name so that multiple values of the same type can coexist. The public API is
// generic and typed end-to-end — there is no untyped registration or lookup:
//
//	c := di.NewContainer()
//	defer func() { _ = c.Close(context.Background()) }()
//
//	// Pre-built value (eager).
//	_ = di.Register(c, logging.NewDefault("app"))
//
//	// Lazily constructed, cached (singleton).
//	_ = di.RegisterSingleton(c, func(ctx context.Context) (*Store, error) { return openStore(ctx) })
//
//	// Fresh instance per resolve (transient).
//	_ = di.RegisterTransient(c, func(ctx context.Context) (*Request, error) { return newRequest(ctx) })
//
//	// Container-owned resource: closed by Close in reverse construction order.
//	_ = di.RegisterSingletonCloseable(c,
//		func(ctx context.Context) (*sql.DB, error) { return sql.Open("pgx", dsn) },
//		func(ctx context.Context, db *sql.DB) error { return db.Close() })
//
//	// Resolution.
//	store, err := di.Resolve[*Store](ctx, c)
//	log := di.MustResolve[*logging.Logger](ctx, c) // panics; startup/tests only
//
// Use [WithName] to register or resolve a named instance:
//
//	_ = di.Register(c, primaryDB, di.WithName("primary"))
//	db, err := di.Resolve[*sql.DB](ctx, c, di.WithName("primary"))
//
// Constructor injection is the only wiring pattern: a factory receives the
// resolution [context.Context] and calls [Resolve] with it for each dependency
// it needs. The active resolution chain travels in that context, so circular
// dependencies are detected and reported as an error, and a canceled context
// aborts resolution.
//
// Resource cleanup is opt-in: only values registered with [RegisterCloseable]
// or [RegisterSingletonCloseable] are released by [Container.Close], which runs
// their disposers in reverse order of construction. Plain [Register] values and
// unresolved singletons are never closed by the container — the caller owns
// them.
package di
