// Package di provides a dependency injection container for gokit applications.
//
// The primary API uses type-safe keys ([Key]) with generics:
//
//	var loggerKey = di.NameKey[*logging.Logger]("logger")
//
//	// Registration
//	di.Provide(c, loggerKey, func() (*logging.Logger, error) { return logging.NewDefault("app"), nil })
//	di.ProvideSingleton(c, loggerKey, existingLogger)
//	di.ProvideTransient(c, loggerKey, func() (*logging.Logger, error) { return logging.NewDefault("app"), nil })
//
//	// Resolution
//	log, err := di.ResolveKey(c, loggerKey)
//	log := di.MustResolveKey(c, loggerKey)  // panics on error; startup/tests only
//
// The container supports eager, lazy, singleton, and transient registration
// modes. Circular dependency detection is built in. Constructor injection is
// the only supported pattern (no setter injection, no service locator).
package di
