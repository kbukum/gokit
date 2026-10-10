// Package jwt issues and verifies JSON Web Tokens for one explicit token profile.
//
// [Config] pins the signing method, issuer, audience set, "typ" header, maximum lifetime and leeway. A [KeySet] supplies
// keys selected by "kid", with per-key retirement for rotation. [Service] fills and checks the registered claims through
// [Registered] and reads time only from an injected clock. Failures are typed: an expired but authentic token reports
// TOKEN_EXPIRED and every other rejection INVALID_TOKEN.
package jwt
