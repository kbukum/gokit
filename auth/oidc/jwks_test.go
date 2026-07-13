package oidc

import (
	"encoding/json"
	"testing"
)

// FuzzJWKSDecode ensures the JWKS JSON decoder never panics on hostile input.
// JWKS is fetched over the network; providers (or attackers via DNS hijack)
// could feed malformed JSON. The decode path must be total.
func FuzzJWKSDecode(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"keys":[]}`))
	f.Add([]byte(`{"keys":[{"kty":"RSA","alg":"RS256","kid":"k1","n":"AQAB","e":"AQAB"}]}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`{"keys":[{"kty":"EC","crv":"P-256","x":"","y":""}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var jwks struct {
			Keys []jwk `json:"keys"`
		}
		_ = json.Unmarshal(data, &jwks)
	})
}
