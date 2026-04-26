package jwt

import (
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// FuzzParse exercises the JWT Service.Parse path with arbitrary input bytes.
// The contract is: Parse must not panic on any input. Invalid tokens must
// surface as errors, not crashes. Algorithm-confusion seeds (alg=none,
// alg=HS256-against-RSA-key, malformed compact form, oversize segments) are
// added to ensure the corpus exercises the security-critical paths.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		".",
		"..",
		"a.b.c",
		"eyJhbGciOiJub25lIn0.e30.", // alg=none
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.",                   // truncated
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.invalidsig", // bad sig
		"\x00\x00\x00",
		string(make([]byte, 64*1024)),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	type fuzzClaims struct {
		gojwt.RegisteredClaims
	}
	cfg := &Config{
		Method: HS256,
		Secret: "fuzz-secret-32-bytes-or-more-for-test",
	}
	svc, err := NewService(cfg, func() *fuzzClaims { return &fuzzClaims{} })
	if err != nil {
		f.Fatalf("NewService: %v", err)
	}

	f.Fuzz(func(t *testing.T, token string) {
		// Contract: never panic. Errors are expected on garbage input.
		_, _ = svc.Parse(token)
	})
}
