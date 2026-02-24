package provider

// Middleware transforms a RequestResponse provider by wrapping it.
// The returned provider typically delegates to the original while
// adding cross-cutting behavior (logging, metrics, tracing, etc.).
type Middleware[I, O any] func(RequestResponse[I, O]) RequestResponse[I, O]

// Chain composes multiple middlewares into one. Middlewares are applied
// in order: the first middleware is outermost (executes first on the
// way in, last on the way out).
//
// Chain(a, b, c)(provider) is equivalent to a(b(c(provider))).
func Chain[I, O any](middlewares ...Middleware[I, O]) Middleware[I, O] {
	return func(inner RequestResponse[I, O]) RequestResponse[I, O] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			inner = middlewares[i](inner)
		}
		return inner
	}
}
