package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwksCache caches JWKS keys with automatic refresh.
type jwksCache struct {
	jwksURI  string
	client   *http.Client
	cacheTTL time.Duration

	mu        sync.RWMutex
	keys      map[string]*jwk
	fetchedAt time.Time
}

// jwk represents a JSON Web Key.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`

	// RSA fields
	N string `json:"n"`
	E string `json:"e"`

	// EC fields
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

func (c *jwksCache) getKey(kid string) (*jwk, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.keys == nil {
		return nil, false
	}
	k, ok := c.keys[kid]
	return k, ok
}

func (c *jwksCache) isStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keys == nil || time.Since(c.fetchedAt) > c.cacheTTL
}

func (c *jwksCache) refresh(ctx context.Context, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURI, http.NoBody)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("JWKS returned %d: %s", resp.StatusCode, string(body))
	}

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make(map[string]*jwk, len(doc.Keys))
	for i := range doc.Keys {
		k := doc.Keys[i]
		if k.Use == "sig" || k.Use == "" {
			keys[k.Kid] = &k
		}
	}

	c.mu.Lock()
	c.keys = keys
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	return nil
}

// getKey retrieves a JWK (not just the public key) so callers can also
// enforce the JWK's declared `alg` matches the token's header alg
// (alg-confusion defense — closes F-002).
func (v *Verifier) getKey(ctx context.Context, kid string) (*jwk, error) {
	// Try cache first
	if !v.jwks.isStale() {
		if k, ok := v.jwks.getKey(kid); ok {
			return k, nil
		}
	}

	// Refresh JWKS
	if err := v.jwks.refresh(ctx, v.config.HTTPClient); err != nil {
		return nil, err
	}

	k, ok := v.jwks.getKey(kid)
	if !ok {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return k, nil
}

// publicKey converts a JWK to a Go crypto.PublicKey.
func (k *jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		return k.rsaPublicKey()
	case "EC":
		return k.ecPublicKey()
	case "OKP":
		return k.okpPublicKey()
	default:
		return nil, fmt.Errorf("unsupported key type: %s", k.Kty)
	}
}

func (k *jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode RSA N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode RSA E: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

func (k *jwk) ecPublicKey() (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("decode EC X: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("decode EC Y: %w", err)
	}

	var curve elliptic.Curve
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve: %s", k.Crv)
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

func (k *jwk) okpPublicKey() (ed25519.PublicKey, error) {
	if k.Crv != "Ed25519" {
		return nil, fmt.Errorf("unsupported OKP curve: %s", k.Crv)
	}
	pubKey, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("decode OKP X: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(pubKey))
	}
	return ed25519.PublicKey(pubKey), nil
}

// --- Signature Verification ---

func verifySignature(rawToken, alg string, key crypto.PublicKey) error {
	parts := splitToken(rawToken)
	if len(parts) != 3 {
		return errors.New("malformed token")
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	switch alg {
	case "RS256":
		return verifyRSA(signingInput, signature, key, crypto.SHA256)
	case "ES256":
		return verifyECDSA(signingInput, signature, key, crypto.SHA256)
	case "EdDSA":
		return verifyEdDSA(signingInput, signature, key)
	default:
		return fmt.Errorf("unsupported algorithm: %s", alg)
	}
}

func verifyRSA(input string, sig []byte, key crypto.PublicKey, hashAlg crypto.Hash) error {
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return errors.New("expected RSA public key")
	}
	h := hashFunc(hashAlg)
	h.Write([]byte(input))
	return rsa.VerifyPKCS1v15(rsaKey, hashAlg, h.Sum(nil), sig)
}

func verifyECDSA(input string, sig []byte, key crypto.PublicKey, hashAlg crypto.Hash) error {
	ecKey, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("expected ECDSA public key")
	}
	h := hashFunc(hashAlg)
	h.Write([]byte(input))

	if !ecdsa.VerifyASN1(ecKey, h.Sum(nil), sig) {
		return errors.New("ECDSA signature verification failed")
	}
	return nil
}

func verifyEdDSA(input string, sig []byte, key crypto.PublicKey) error {
	edKey, ok := key.(ed25519.PublicKey)
	if !ok {
		return errors.New("expected Ed25519 public key")
	}
	if !ed25519.Verify(edKey, []byte(input), sig) {
		return errors.New("Ed25519 signature verification failed")
	}
	return nil
}

func hashFunc(alg crypto.Hash) hash.Hash {
	switch alg {
	case crypto.SHA256:
		return sha256.New()
	case crypto.SHA384:
		return sha512.New384()
	case crypto.SHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

func splitToken(raw string) []string {
	idx1 := indexOf(raw, '.', 0)
	if idx1 < 0 {
		return nil
	}
	idx2 := indexOf(raw, '.', idx1+1)
	if idx2 < 0 {
		return nil
	}
	return []string{raw[:idx1], raw[idx1+1 : idx2], raw[idx2+1:]}
}

func indexOf(s string, c byte, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// decodeJWTSegment decodes a base64url-encoded JWT segment into a map.
func decodeJWTSegment(seg string) (map[string]interface{}, error) {
	data, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
