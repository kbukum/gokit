// Package bootstrap orchestrates application lifecycle for gokit services.
//
// It provides typed configuration loading, component registration, dependency
// injection, and startup/shutdown hooks for rapid service initialization.
//
// Two execution modes are supported:
//
//   - Run: for long-running services that block until a shutdown signal
//   - RunTask: for CLI tools and batch jobs that execute a finite task
//
// # Server Example
//
//	app, _ := bootstrap.NewApp(&cfg)
//	app.OnConfigure(func(ctx context.Context, a *bootstrap.App[*MyConfig]) error {
//	    // wire up services, routes, etc.
//	    return nil
//	})
//	app.Run(ctx) // blocks until SIGINT/SIGTERM
//
// # Task Example
//
//	app, _ := bootstrap.NewApp(&cfg)
//	app.RunTask(ctx, func(ctx context.Context) error {
//	    return processData(ctx) // runs to completion, then shuts down
//	})
package bootstrap
