// Package redis provides a Redis client component with connection pooling,
// lifecycle management, and health checks for gokit applications.
//
// It wraps go-redis with gokit logging, configuration conventions, and
// component lifecycle (Init/Start/Stop/Health) for cache and session
// storage operations.
//
// # Quick Start
//
//	cfg := redis.Config{
//	    Host: "localhost",
//	    Port: 6379,
//	}
//	component := redis.NewComponent(cfg)
package redis
