// Package jwt parses and verifies JSON Web Tokens with an explicit, allow-listed signing method.
//
// [Config] pins the accepted [SigningMethod] and secret/key material so a token signed with an unexpected algorithm is rejected rather than silently trusted. Parsing returns typed errors for unsupported algorithms and malformed tokens.
package jwt
