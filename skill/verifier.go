package skill

// VerificationStatus is the outcome kind of a manifest verification.
type VerificationStatus int

const (
	// VerificationVerified indicates the manifest passed verification.
	VerificationVerified VerificationStatus = iota
	// VerificationWarning indicates verification succeeded with non-fatal warnings.
	VerificationWarning
	// VerificationDenied indicates verification rejected the manifest.
	VerificationDenied
)

// VerificationOutcome is the result of verifying a manifest at load time.
type VerificationOutcome struct {
	Status   VerificationStatus
	Warnings []string
	Reason   string
}

// Verified reports a passing verification with no warnings.
func Verified() VerificationOutcome { return VerificationOutcome{Status: VerificationVerified} }

// Warning reports a passing verification carrying non-fatal warnings.
func Warning(messages ...string) VerificationOutcome {
	return VerificationOutcome{Status: VerificationWarning, Warnings: messages}
}

// Denied reports a rejected verification with a human-readable reason.
func Denied(reason string) VerificationOutcome {
	return VerificationOutcome{Status: VerificationDenied, Reason: reason}
}

// Verifier verifies a skill manifest at load time. The loader consults it after
// parsing and validation: Denied fails the load, Warning is surfaced on the
// pack, and Verified proceeds silently.
//
// Implementations MUST be safe for concurrent use.
type Verifier interface {
	Verify(manifest *Manifest, root string) (VerificationOutcome, error)
}

// WarnOnlyVerifier permits unsigned manifests with a warning and treats any
// signed manifest as verified. Suitable for development and tests; operators
// SHOULD pair it with DenyVerifier or a real signature verifier in production.
type WarnOnlyVerifier struct{}

// Verify returns Verified for signed manifests and a Warning otherwise.
func (WarnOnlyVerifier) Verify(manifest *Manifest, _ string) (VerificationOutcome, error) {
	if manifest != nil && manifest.Signature != nil {
		return Verified(), nil
	}
	return Warning("unsigned skill manifest"), nil
}

// DenyVerifier is the canonical operator-deny verifier: it rejects every
// manifest unconditionally. Use it as a safe default until a real signature
// verifier (e.g., Sigstore/cosign) is wired in.
type DenyVerifier struct{}

// Verify always denies.
func (DenyVerifier) Verify(*Manifest, string) (VerificationOutcome, error) {
	return Denied("deny verifier: signatures rejected"), nil
}
