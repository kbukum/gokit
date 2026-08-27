package providers

import "io"

// maxResponseBytes bounds how much of an identity-provider HTTP response body is
// read. The IdP is a trust boundary; capping the read stops a hostile or
// misbehaving provider from exhausting memory with an unbounded body. 1 MiB
// comfortably fits token and userinfo payloads.
const maxResponseBytes = 1 << 20

// readResponseBody reads at most [maxResponseBytes] from an IdP response body.
func readResponseBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxResponseBytes))
}
