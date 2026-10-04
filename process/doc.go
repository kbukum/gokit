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
// Persistent lifetime capture defaults to 64 KiB per stream. Shutdown returns forced
// termination as an error, distinguishes cancellation/deadline, and exposes Complete
// for group release and completed Wait/output collection. An unreaped leader reserves
// its original group identity until cleanup; released groups are never reacquired by
// numeric PID. Incomplete cleanup stays owned for retry, preserving historical errors.
// StartPersistent can return an owning run with an error when failed-start cleanup is
// incomplete; callers must retain and clean that lease.
// Supervisor reports every child across partial retries. Wait observes exit without
// stopping a live child, then drains and reaps. Owned observation supports Linux,
// Darwin, and Windows; Windows termination is forced and bare-PID cleanup unsupported.
//
// Pseudoterminal (PTY) execution is intentionally not provided here: it is a heavy, Unix-only
// capability that would pull a platform dependency into the root module.
package process
