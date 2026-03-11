package provider

// SinkMiddleware transforms a Sink provider by wrapping it.
// The returned Sink typically delegates to the original while
// adding cross-cutting behavior (logging, metrics, tracing, etc.).
type SinkMiddleware[I any] func(Sink[I]) Sink[I]

// ChainSink composes multiple sink middlewares into one. Middlewares are applied
// in order: the first middleware is outermost (executes first on the
// way in, last on the way out).
//
// ChainSink(a, b, c)(sink) is equivalent to a(b(c(sink))).
func ChainSink[I any](middlewares ...SinkMiddleware[I]) SinkMiddleware[I] {
	return func(inner Sink[I]) Sink[I] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			inner = middlewares[i](inner)
		}
		return inner
	}
}
