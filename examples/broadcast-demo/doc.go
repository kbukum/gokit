// Command broadcast-demo is a runnable example that consumes stream.Broadcaster as a
// real subscriber would: it fans a sequence of configuration-change events out to
// several independent subscribers, deliberately overruns one deliberately-slow
// subscriber to exercise backpressure-by-drop, and bridges the broadcaster's OnDrop
// hook to an observability counter so lost events are observable.
//
// It is a demonstration, not a supported API surface. The testable core lives in
// broadcast.go; main.go is a thin shell that wires a logger and meter and prints a
// short summary.
package main
