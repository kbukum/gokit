// Package lease keeps long-lived work alive only while an injected authority keeps confirming it.
//
// A [Set] holds application-keyed leases. One owned worker checks distinct keys in bounded batches; [Set.Nudge] coalesces requests for early renewal. Timely confirmations extend from check start, denials end leases and unanswered keys expire. Partial answers remain authoritative alongside reported errors. A separate expiry worker prevents stalled checks or late replies from extending authority. [Config.Clock] injects elapsed-time scheduling and defaults to monotonic time. Close accounts for every checker call; the package knows nothing about credentials or tenants.
package lease
