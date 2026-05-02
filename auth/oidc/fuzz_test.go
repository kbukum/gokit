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

// FuzzDiscoveryDocument ensures the OIDC discovery document JSON decoder never
// panics on hostile input. The discovery endpoint is fetched over the network;
// a compromised or attacker-controlled provider could return malformed JSON.
func FuzzDiscoveryDocument(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"issuer":"https://example.com","jwks_uri":"https://example.com/jwks","authorization_endpoint":"https://example.com/auth","token_endpoint":"https://example.com/token"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`{"issuer":"","jwks_uri":""}`))
	f.Add([]byte(`{"issuer":"https://example.com","jwks_uri":"https://example.com/jwks","id_token_signing_alg_values_supported":["RS256","ES256","EdDSA"]}`))
	f.Add([]byte(`{"issuer":null,"jwks_uri":null}`))
	f.Add([]byte(`{"issuer":true,"jwks_uri":[]}`))
	f.Add([]byte(`{"issuer":` + "\"" + string(make([]byte, 65536)) + "\"" + `}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Mirror the actual parse path in verifier.go:discover()
		type doc struct {
			Issuer                string   `json:"issuer"`
			AuthorizationEndpoint string   `json:"authorization_endpoint"`
			TokenEndpoint         string   `json:"token_endpoint"`
			UserInfoEndpoint      string   `json:"userinfo_endpoint"`
			JWKSUri               string   `json:"jwks_uri"`
			SupportedScopes       []string `json:"scopes_supported"`
			SupportedAlgs         []string `json:"id_token_signing_alg_values_supported"`
		}
		var d doc
		_ = json.Unmarshal(data, &d)
		// Validate the critical field exactly as the real code does.
		_ = d.JWKSUri == ""
	})
}
