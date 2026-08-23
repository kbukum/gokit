// Package process provides subprocess execution with context cancellation, signal handling,
// and structured output capture.
//
// Run executes a command and waits for it, capturing stdout and stderr into bounded buffers;
// Stream observes output live through a callback while the process runs. Both classify a
// failure to spawn via SpawnError (a missing executable becomes NotFound, a permission
// failure Forbidden, anything else Internal) and record timeout or cancellation on the
// returned Result, whose Check reports the outcome as a typed error.
//
// IO modes select how standard streams are wired: captured (the default), observed (Stream),
// or inherited from the parent terminal, with a stdin policy of closed, provided bytes, or
// inherited. LifecyclePolicy governs process-group isolation and graceful-termination
// escalation for every spawn.
//
// Supervisor tracks live children and tears them all down on Shutdown, escalating to a force
// kill after the grace period. StartPersistent runs a long-lived subprocess with readiness
// detection (immediate, on an output marker, or after a delay) and graceful shutdown; a
// startup failure carries a machine-readable classification retrievable via StartErrorKind.
// The InterruptGroup, TerminateGroup, and KillGroup helpers signal a command's process group.
//
// Pseudoterminal (PTY) execution is intentionally not provided here: it is a heavy, Unix-only
// capability that would pull a platform dependency into the root module.
package process
