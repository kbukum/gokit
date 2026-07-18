// Package signal maps interactive interrupts (Ctrl+C / SIGTERM) onto cooperative [context.Context] cancellation.
//
// Where rskit standardizes on a cancellation token, Go's idiom is a cancelable context threaded through every remote call and goroutine. This package builds that context from OS signals so a CLI winds down gracefully on the first interrupt: spawned work observes ctx.Done() and drains. Calling the returned stop function restores default signal handling, so a later interrupt terminates the process immediately.
package signal
