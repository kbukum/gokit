// Package testutil provides an in-process gRPC server harness for
// deterministic, network-free transport tests.
//
// The [Server] runs a real grpc.Server over an in-memory bufconn listener, so
// tests exercise the genuine gRPC client stack — dialing, interceptors,
// deadlines, cancellation, and status propagation — without opening a socket or
// binding a port. A single configurable unary method lets a test drive the
// server's response: return success, block until the caller's context is done,
// sleep to force a deadline, or return any gRPC status (including one produced
// by the canonical AppError mapping) so the client-side mapping can be proven
// end to end.
//
// Server implements component.Component and testutil.TestComponent, matching the
// shape of the other gokit test harnesses; extend it here rather than
// hand-rolling a fake gRPC server inside a _test.go file.
package testutil
