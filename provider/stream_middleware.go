package provider

// StreamMiddleware transforms a Stream provider by wrapping it.
// The returned Stream typically delegates to the original while adding cross-cutting behavior (logging, metrics, tracing, etc.).
type StreamMiddleware[I, O any] func(Stream[I, O]) Stream[I, O]

// ChainStream composes multiple stream middlewares into one. Middlewares are applied in order:
// the first middleware is outermost (executes first on the way in, last on the way out).
//
// ChainStream(a, b, c)(stream) is equivalent to a(b(c(stream))).
func ChainStream[I, O any](middlewares ...StreamMiddleware[I, O]) StreamMiddleware[I, O] {
	return func(inner Stream[I, O]) Stream[I, O] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			inner = middlewares[i](inner)
		}
		return inner
	}
}
