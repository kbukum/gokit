package oidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
)

// makeJWKS creates an in-process jwksCache populated with n RSA keys; it
// avoids any HTTP I/O so the benchmark measures only the lookup/verify hot
// paths.
func makeJWKS(b *testing.B, n int) (*jwksCache, []*rsa.PrivateKey, []string) {
	b.Helper()
	c := &jwksCache{keys: make(map[string]*jwk, n)}
	keys := make([]*rsa.PrivateKey, n)
	kids := make([]string, n)
	for i := 0; i < n; i++ {
		pk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			b.Fatalf("gen key: %v", err)
		}
		kid := "k" + strconv.Itoa(i)
		c.keys[kid] = &jwk{
			Kty: "RSA",
			Kid: kid,
			Alg: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(pk.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big2bytes(pk.E)),
		}
		keys[i] = pk
		kids[i] = kid
	}
	return c, keys, kids
}

func big2bytes(e int) []byte {
	if e == 0 {
		return []byte{0}
	}
	b := make([]byte, 0, 4)
	for e > 0 {
		b = append([]byte{byte(e & 0xff)}, b...)
		e >>= 8
	}
	return b
}

func BenchmarkJWKS_GetKey_Hit(b *testing.B) {
	const n = 8
	c, _, kids := makeJWKS(b, n)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.getKey(kids[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkJWKS_GetKey_Miss(b *testing.B) {
	c, _, _ := makeJWKS(b, 8)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.getKey("nope"); ok {
			b.Fatal("hit")
		}
	}
}

func BenchmarkJWKS_PublicKey(b *testing.B) {
	c, _, kids := makeJWKS(b, 1)
	k, _ := c.getKey(kids[0])

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := k.publicKey(); err != nil {
			b.Fatalf("publicKey: %v", err)
		}
	}
}

func BenchmarkVerifyRSA_RS256(b *testing.B) {
	c, keys, kids := makeJWKS(b, 1)
	jk, _ := c.getKey(kids[0])
	pub, err := jk.publicKey()
	if err != nil {
		b.Fatalf("publicKey: %v", err)
	}

	signingInput := "header.payload"
	h := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, keys[0], crypto.SHA256, h[:])
	if err != nil {
		b.Fatalf("sign: %v", err)
	}

	// Validate setup once.
	if _, ok := pub.(*rsa.PublicKey); !ok {
		b.Fatal("not rsa key")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := verifyRSA(signingInput, sig, pub, crypto.SHA256); err != nil {
			b.Fatalf("verify: %v", err)
		}
	}
}
