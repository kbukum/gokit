// Package testutil provides testing utilities for the redis module.
//
// It includes an in-memory Redis test component using miniredis that
// implements both component.Component and testutil.TestComponent interfaces.
//
// # Quick Start
//
//	redis := testutil.NewComponent()
//	testutil.T(t).Setup(redis)
//
//	// Use redis.Client() to access the go-redis client
//	redis.Client().Set(ctx, "key", "value", 0)
//
// # State Management
//
//	testutil.T(t).Reset(redis)   // Flushes all keys
package testutil
