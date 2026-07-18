package messaging

// HandlerMiddleware transforms a MessageHandler by wrapping it with additional behavior (logging, metrics, retry, tracing, etc.).
type HandlerMiddleware func(MessageHandler) MessageHandler

// ChainHandlers composes middlewares around a base handler. Middlewares are applied
// so that the first element in the slice is the outermost wrapper (executes first on the way in, last on the way out).
//
//	chain := ChainHandlers(base, logging, metrics, retry)
//	// execution order: logging → metrics → retry → base
func ChainHandlers(base MessageHandler, middlewares ...HandlerMiddleware) MessageHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		base = middlewares[i](base)
	}
	return base
}
