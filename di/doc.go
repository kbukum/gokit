// Package di provides a dependency injection container for gokit applications.
//
// It supports eager, lazy, and singleton registration modes with type-safe
// resolution using Go generics. The container enables decoupled architecture
// by managing service dependencies and their lifecycle.
//
// # Registration
//
//	di.Register[MyService](container, func() (*MyService, error) {
//	    return NewMyService(), nil
//	})
//
// # Resolution
//
//	svc := di.MustResolve[MyService](container)
package di
