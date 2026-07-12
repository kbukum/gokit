// Package di provides a small, type-keyed dependency injection container.
//
// Dependencies are keyed by their concrete Go type, optionally qualified by a
// name so that multiple values of the same type can coexist. The public API is
// generic and typed end-to-end — there is no untyped registration or lookup:
//
//	c := di.NewContainer()
//	defer func() { _ = c.Close() }()
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
// aborts resolution. Eager registrations and resolved singletons whose value
// implements interface{ Close() error } are closed by [Container.Close];
// transients and unresolved singletons are never stored, so nothing is closed
// for them.
package di
