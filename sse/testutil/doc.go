// Package testutil provides an automatically cleaned-up HTTP Harness, StreamClient, FakeAuthenticator, and deadline-aware ResponseWriter for SSE endpoint tests.
//
// New accepts explicit bus limits and handler configuration. Harness.Resume sends an acknowledged cursor in Last-Event-ID. StreamClient uses the canonical bounded Decoder; controls may inherit the parser's previous ID and must never advance application acknowledgement.
package testutil
