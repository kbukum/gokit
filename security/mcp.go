package security

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type Verifier interface {
	Verify(ctx context.Context, payload []byte, signature []byte) error
}
type WarnOnlyVerifier struct{}

func (WarnOnlyVerifier) Verify(context.Context, []byte, []byte) error { return nil }

func ValidateLocalBind(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return fmt.Errorf("security: streamable_http bind host must be explicit localhost")
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return fmt.Errorf("security: streamable_http bind must be localhost by default")
}

func ValidateOrigin(r *http.Request, allowed []string) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	for _, a := range allowed {
		if strings.EqualFold(origin, a) {
			return nil
		}
	}
	return fmt.Errorf("security: origin %q is not allowed", origin)
}

func EnforcePayloadLimit(w http.ResponseWriter, r *http.Request, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	if r.ContentLength > maxBytes {
		return fmt.Errorf("security: payload exceeds %d bytes", maxBytes)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return nil
}
