// Package signal maps interactive interrupts (Ctrl+C / SIGTERM) onto cooperative
// [context.Context] cancelation.
//
// Where rskit standardizes on a cancelation token, Go's idiom is a cancelable
// context threaded through every remote call and goroutine. This package builds
// that context from OS signals so a CLI winds down gracefully on the first
// interrupt: spawned work observes ctx.Done() and drains, and a second interrupt
// restores the default behavior (immediate termination).
package signal
