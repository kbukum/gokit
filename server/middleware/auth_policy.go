package middleware

// MissingTokenPolicy governs how the auth middleware treats a request that carries no credentials.
// It never affects invalid credentials: a credential that is present
// but fails validation is always rejected, regardless of policy.
// It expresses the AcceptMissing / reject-invalid split explicitly.
type MissingTokenPolicy int

const (
	// RejectMissing rejects requests without credentials (the secure default, and the zero value).
	// Used by Auth and by default in HTTPAuth.
	RejectMissing MissingTokenPolicy = iota

	// AcceptMissing lets unauthenticated requests proceed while still rejecting any present-but-invalid token.
	// Used by OptionalAuth and HTTPAuth with WithMissingPolicy.
	AcceptMissing
)

// String implements fmt.Stringer for readable test and log output.
func (p MissingTokenPolicy) String() string {
	switch p {
	case AcceptMissing:
		return "AcceptMissing"
	default:
		return "RejectMissing"
	}
}
