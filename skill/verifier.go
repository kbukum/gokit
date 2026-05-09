package skill

import (
	"context"
	"errors"
	"log/slog"
)

// ErrSignatureRejected is returned by DenyVerifier and signals that the
// operator has chosen to reject all skill manifests until a real signature
// verifier (Sigstore/cosign) is wired in.
var ErrSignatureRejected = errors.New("skill: deny verifier rejects all signatures")

// Verifier validates a skill manifest's signature.
//
// Implementations MUST be safe for concurrent use.
type Verifier interface {
	Verify(manifestBytes []byte, sig Signature) error
}

// WarnOnlyVerifier permits unsigned manifests with a warning log. Suitable
// for development and tests; operators SHOULD pair it with DenyVerifier or a
// real signature verifier in production.
type WarnOnlyVerifier struct{ Logger *slog.Logger }

// Verify logs a warning when no signature is present and otherwise allows.
func (v WarnOnlyVerifier) Verify(manifestBytes []byte, sig Signature) error {
	if sig.Value == "" && v.Logger != nil {
		v.Logger.WarnContext(context.Background(), "skill manifest signature missing", "manifest_bytes", len(manifestBytes))
	}
	return nil
}

// DenyVerifier is the canonical operator-deny verifier: it rejects every
// manifest unconditionally. Use it as a safe default until a real signature
// verifier (e.g., Sigstore/cosign) is wired in.
type DenyVerifier struct{}

// Verify always returns ErrSignatureRejected.
func (DenyVerifier) Verify(_ []byte, _ Signature) error { return ErrSignatureRejected }
