package auth

// TokenProvider resolves a token at call time.
type TokenProvider interface {
	Token() (string, error)
}
