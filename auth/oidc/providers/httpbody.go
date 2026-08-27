package providers

import (
	"fmt"
	"io"
)

// maxResponseBytes bounds how much of an identity-provider HTTP response body is
// read. The IdP is a trust boundary; capping the read stops a hostile or
// misbehaving provider from exhausting memory with an unbounded body. 1 MiB
// comfortably fits token and userinfo payloads.
const maxResponseBytes = 1 << 20

// ErrResponseTooLarge is returned when an IdP response exceeds [maxResponseBytes].
// The whole response is rejected rather than parsed from a truncated prefix,
// because a silently cut payload can still be syntactically valid JSON (a
// complete object followed by extra data) and would otherwise be accepted.
var ErrResponseTooLarge = fmt.Errorf("provider response exceeds %d bytes", maxResponseBytes)

// readResponseBody reads at most [maxResponseBytes] from an IdP response body,
// returning [ErrResponseTooLarge] when the body is larger. It reads one extra
// byte so oversize is detected explicitly instead of truncating to a valid prefix.
func readResponseBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}
