package messaging

import (
	"context"
	"strings"
	"sync"
)

// RouterOption configures Router behavior.
type RouterOption func(*routerConfig)

type routerConfig struct {
	keyFunc func(Message) string
}

// WithRouterKeyFunc overrides the default routing key extractor. By default,
// the message topic is used as the routing key.
func WithRouterKeyFunc(fn func(Message) string) RouterOption {
	return func(c *routerConfig) { c.keyFunc = fn }
}

// route associates a pattern with its handler and precomputed prefix (for wildcards).
type route struct {
	pattern string
	handler MessageHandler
	prefix  string // non-empty when pattern ends with ".*"
}

// Router routes incoming messages to handlers based on topic or custom key.
// It supports exact match, wildcard patterns (e.g. "content.*"), and a default fallback handler.
// Router is safe for concurrent use.
type Router struct {
	mu       sync.RWMutex
	routes   []route
	fallback MessageHandler
	opts     []RouterOption
}

// NewRouter creates a new Router.
func NewRouter(opts ...RouterOption) *Router {
	return &Router{opts: opts}
}

// Handle registers a handler for the given pattern.
// Patterns support exact match ("content.discovered")
// or wildcard ("content.*") where "*" matches any suffix after the last dot.
func (r *Router) Handle(pattern string, handler MessageHandler) *Router {
	r.mu.Lock()
	defer r.mu.Unlock()

	rt := route{pattern: pattern, handler: handler}
	if strings.HasSuffix(pattern, ".*") {
		rt.prefix = strings.TrimSuffix(pattern, "*")
	}
	r.routes = append(r.routes, rt)
	return r
}

// Default sets the fallback handler for messages that match no registered pattern.
func (r *Router) Default(handler MessageHandler) *Router {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = handler
	return r
}

// Handler returns a MessageHandler that routes messages based on registered patterns.
// The routing key is the message topic by default; use WithRouterKeyFunc to override.
func (r *Router) Handler() MessageHandler {
	cfg := routerConfig{
		keyFunc: func(msg Message) string { return msg.Topic },
	}
	for _, opt := range r.opts {
		opt(&cfg)
	}

	return func(ctx context.Context, msg Message) error {
		key := cfg.keyFunc(msg)

		r.mu.RLock()
		defer r.mu.RUnlock()

		// Exact match first, then wildcard.
		for i := range r.routes {
			rt := &r.routes[i]
			if rt.prefix == "" && rt.pattern == key {
				return rt.handler(ctx, msg)
			}
		}
		for i := range r.routes {
			rt := &r.routes[i]
			if rt.prefix != "" && strings.HasPrefix(key, rt.prefix) {
				return rt.handler(ctx, msg)
			}
		}

		if r.fallback != nil {
			return r.fallback(ctx, msg)
		}
		return nil
	}
}
