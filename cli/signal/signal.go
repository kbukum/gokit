package signal

import (
	"context"
	"os"
	"os/signal"
)

// InterruptSignals returns the OS signals treated as a graceful-shutdown request:
// [os.Interrupt] (Ctrl+C) everywhere,
// plus SIGTERM on platforms that deliver it (Windows has no SIGTERM analog).
//
// It returns a fresh slice on each call so callers cannot mutate shared state.
func InterruptSignals() []os.Signal {
	extra := terminationSignals()
	sigs := make([]os.Signal, 0, 1+len(extra))
	sigs = append(sigs, os.Interrupt)
	return append(sigs, extra...)
}

// NotifyContext returns a copy of parent that is canceled when one of sigs is delivered.
//
// It wraps [signal.NotifyContext]: the first matching signal cancels the returned context,
// and the returned stop function both releases the signal handler and cancels the context.
// Until stop runs the handler stays installed,
// so a second signal is absorbed rather than terminating;
// call stop once shutdown begins to let a later interrupt force-exit,
// and always call it to avoid leaking the handler.
// With no sigs the context is canceled only by stop or when parent is.
func NotifyContext(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, sigs...)
}

// OnInterrupt returns a context canceled on the first interrupt
// or termination signal (SIGINT / SIGTERM).
//
// It is the common case of [NotifyContext]: hand the returned context to spawned tasks
// and remote calls so they wind down on Ctrl+C,
// and call the returned stop function on shutdown to release the handler.
func OnInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	return NotifyContext(parent, InterruptSignals()...)
}
