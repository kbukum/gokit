package middleware

// MissingTokenPolicy governs how the auth middleware treats a request that carries no credentials. It never affects invalid credentials: a token that is present but fails validation is always rejected with 401, regardless of policy. This mirrors rskit's MissingCredentialPolicy so both kits express the AcceptMissing / reject-invalid split identically.
type MissingTokenPolicy int

const (
	// RejectMissing rejects requests without credentials (the secure default, and the zero value). Used by Auth.
	RejectMissing MissingTokenPolicy = iota

	// AcceptMissing lets unauthenticated requests proceed while still rejecting any present-but-invalid token. Used by OptionalAuth.
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
