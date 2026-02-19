// Package bootstrap orchestrates application lifecycle for gokit services.
//
// It provides typed configuration loading, component registration, dependency
// injection, and startup/shutdown hooks for rapid service initialization.
//
// # Quick Start
//
//	app := bootstrap.New[MyConfig]("my-service",
//	    bootstrap.WithComponent(dbComponent),
//	    bootstrap.WithComponent(serverComponent),
//	)
//	if err := app.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// The bootstrap package handles configuration loading, component initialization
// in dependency order, graceful shutdown on OS signals, and health aggregation.
package bootstrap
