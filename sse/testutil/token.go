package testutil

// TokenValidator returns programmed opaque claims or a diagnostic error for bearer-authentication tests.
type TokenValidator struct {
	Claims any
	Err    error
}

func (v TokenValidator) ValidateToken(string) (any, error) { return v.Claims, v.Err }
