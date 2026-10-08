// Package bootstrap orchestrates application lifecycle for gokit services.
//
// It provides typed configuration loading, component registration, dependency injection,
// and startup/shutdown hooks for rapid service initialization.
//
// Two execution modes are supported:
//
//   - Run: for long-running services that block until a shutdown signal
//   - RunTask: for CLI tools and batch jobs that execute a finite task
//
// # Modules
//
// A [Module] declares the typed [Port] values it provides and needs. Commands compose modules with [App.Use] and declare named HTTP listeners with [App.RegisterListener], before Run or in a configure hook. A module that runs in another service is replaced by its client module, which provides the same ports remotely, and [ValueModule] provides a port with a value the command holds, such as a test double. After configure hooks run, startup checks the whole module set and reports every wiring problem in one [*ModuleError], then registers modules in dependency order; [App.CheckModules] runs the same check without starting anything. Package bootstrap/testutil runs modules in tests and checks that a module and its client behave the same.
//
// # Server Example
//
//	app, err := bootstrap.NewApp(&cfg)
//	if err != nil {
//	    return err
//	}
//	app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
//	    // wire up services, routes, etc.
//	    return nil
//	})
//	return app.Run(ctx) // blocks until SIGINT/SIGTERM
//
// # Task Example
//
//	app, err := bootstrap.NewApp(&cfg)
//	if err != nil {
//	    return err
//	}
//	return app.RunTask(ctx, func(ctx context.Context) error {
//	    return processData(ctx) // runs to completion, then shuts down
//	})
package bootstrap
