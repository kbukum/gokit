// Package live provides a bounded, multi-region live console for streaming
// several concurrent outputs as fixed-height tiles.
//
// [Console] stacks each [Region] as a bounded tile in a live area that is
// redrawn in place. The tiles are an ephemeral peek: each region retains only
// its most recent lines (a documented per-region bound), and the number of
// regions is capped (a documented backpressure bound), so a flood of transient
// progress never grows without limit. Durable signal — a region's final verdict
// — is written to scrollback above the live area on [Region.Finish].
//
// Rendering is deterministic and clock-free: the console redraws only when the
// caller calls [Console.Render], writing to an injected [io.Writer], so it is
// fully testable without a real terminal.
package live
