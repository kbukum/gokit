// Package testutil provides testing utilities for the storage module.
//
// It includes an in-memory storage component that implements both
// component.Component and testutil.TestComponent interfaces.
//
// # Quick Start
//
//	store := testutil.NewComponent()
//	testutil.T(t).Setup(store)
//
//	// Use store.Storage() to access the storage.Storage interface
//	store.Storage().Upload(ctx, "path/file.txt", strings.NewReader("hello"))
package testutil
