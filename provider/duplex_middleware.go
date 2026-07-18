package provider

// DuplexMiddleware transforms a Duplex provider by wrapping it.
// The returned Duplex typically delegates to the original while adding cross-cutting behavior (logging, metrics, tracing, etc.).
type DuplexMiddleware[I, O any] func(Duplex[I, O]) Duplex[I, O]

// ChainDuplex composes multiple duplex middlewares into one. Middlewares are applied in order:
// the first middleware is outermost (executes first on the way in, last on the way out).
//
// ChainDuplex(a, b, c)(duplex) is equivalent to a(b(c(duplex))).
func ChainDuplex[I, O any](middlewares ...DuplexMiddleware[I, O]) DuplexMiddleware[I, O] {
	return func(inner Duplex[I, O]) Duplex[I, O] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			inner = middlewares[i](inner)
		}
		return inner
	}
}
