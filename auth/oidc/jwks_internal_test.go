package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJWKSRefreshFailureDoesNotStartForcedCooldown(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := &jwksCache{
		jwksURI:  srv.URL,
		cacheTTL: time.Hour,
		keys:     map[string]*jwk{"existing": {Kid: "existing", Kty: "RSA"}},
	}

	client := srv.Client()
	if err := cache.refresh(context.Background(), client, true); err == nil {
		t.Fatal("expected forced refresh to fail")
	}
	if !cache.lastForcedRefreshAt.IsZero() {
		t.Fatal("failed refresh should not update forced refresh cooldown")
	}
	if err := cache.refresh(context.Background(), client, true); err == nil {
		t.Fatal("expected second forced refresh to fail")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected second forced refresh attempt without cooldown, got %d hits", got)
	}
}

func TestJWKSForcedRefreshIsSerialized(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kid":"rsa-1","kty":"RSA","n":"AQAB","e":"AQAB","use":"sig"}]}`))
	}))
	defer srv.Close()

	cache := &jwksCache{
		jwksURI:  srv.URL,
		cacheTTL: time.Hour,
		keys:     map[string]*jwk{"existing": {Kid: "existing", Kty: "RSA"}},
	}

	const callers = 4
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- cache.refresh(context.Background(), srv.Client(), true)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("forced refresh failed: %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected exactly one forced refresh request, got %d", got)
	}
}

func rsaJWK(t *testing.T, pub *rsa.PublicKey) *jwk {
	t.Helper()
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	return &jwk{
		Kty: "RSA",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}

func TestPublicKeyConversions(t *testing.T) {
	t.Parallel()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	if _, err := rsaJWK(t, &rsaKey.PublicKey).publicKey(); err != nil {
		t.Fatalf("RSA publicKey: %v", err)
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa keygen: %v", err)
	}
	ecJWK := &jwk{
		Kty: "EC", Crv: "P-256",
		X: base64.RawURLEncoding.EncodeToString(ecKey.X.Bytes()),
		Y: base64.RawURLEncoding.EncodeToString(ecKey.Y.Bytes()),
	}
	if _, err := ecJWK.publicKey(); err != nil {
		t.Fatalf("EC publicKey: %v", err)
	}

	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	okpJWK := &jwk{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(edPub)}
	if _, err := okpJWK.publicKey(); err != nil {
		t.Fatalf("OKP publicKey: %v", err)
	}

	if _, err := (&jwk{Kty: "UNKNOWN"}).publicKey(); err == nil {
		t.Fatal("expected unsupported key type error")
	}
}

func TestPublicKeyConversionErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]*jwk{
		"rsa-bad-n":     {Kty: "RSA", N: "!!!", E: "AQAB"},
		"rsa-bad-e":     {Kty: "RSA", N: "AQAB", E: "!!!"},
		"ec-bad-x":      {Kty: "EC", Crv: "P-256", X: "!!!", Y: "AQAB"},
		"ec-bad-y":      {Kty: "EC", Crv: "P-256", X: "AQAB", Y: "!!!"},
		"ec-bad-curve":  {Kty: "EC", Crv: "P-999", X: "AQAB", Y: "AQAB"},
		"okp-bad-curve": {Kty: "OKP", Crv: "X25519", X: "AQAB"},
		"okp-bad-x":     {Kty: "OKP", Crv: "Ed25519", X: "!!!"},
		"okp-short-x":   {Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString([]byte("short"))},
	}
	for name, k := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := k.publicKey(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestVerifySignatureRoundTrips(t *testing.T) {
	t.Parallel()
	input := "header.payload"

	t.Run("RS256", func(t *testing.T) {
		t.Parallel()
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		sum := sha256.Sum256([]byte(input))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		token := input + "." + base64.RawURLEncoding.EncodeToString(sig)
		if err := verifySignature(token, "RS256", &key.PublicKey); err != nil {
			t.Fatalf("verify RS256: %v", err)
		}
		if err := verifySignature(token, "RS256", "not-a-key"); err == nil {
			t.Fatal("expected RSA key type error")
		}
	})

	t.Run("ES256", func(t *testing.T) {
		t.Parallel()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		sum := sha256.Sum256([]byte(input))
		sig, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		token := input + "." + base64.RawURLEncoding.EncodeToString(sig)
		if err := verifySignature(token, "ES256", &key.PublicKey); err != nil {
			t.Fatalf("verify ES256: %v", err)
		}
		badSum := sha256.Sum256([]byte("other"))
		badSig, _ := ecdsa.SignASN1(rand.Reader, key, badSum[:])
		badToken := input + "." + base64.RawURLEncoding.EncodeToString(badSig)
		if err := verifySignature(badToken, "ES256", &key.PublicKey); err == nil {
			t.Fatal("expected ES256 verification failure")
		}
		if err := verifySignature(token, "ES256", "not-a-key"); err == nil {
			t.Fatal("expected ECDSA key type error")
		}
	})

	t.Run("EdDSA", func(t *testing.T) {
		t.Parallel()
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		sig := ed25519.Sign(priv, []byte(input))
		token := input + "." + base64.RawURLEncoding.EncodeToString(sig)
		if err := verifySignature(token, "EdDSA", pub); err != nil {
			t.Fatalf("verify EdDSA: %v", err)
		}
		if err := verifySignature(token, "EdDSA", "not-a-key"); err == nil {
			t.Fatal("expected Ed25519 key type error")
		}
	})
}

func TestVerifySignatureErrors(t *testing.T) {
	t.Parallel()
	if err := verifySignature("only.two", "RS256", nil); err == nil {
		t.Fatal("expected malformed token error")
	}
	if err := verifySignature("a.b.!!!", "RS256", nil); err == nil {
		t.Fatal("expected signature decode error")
	}
	if err := verifySignature("a.b."+base64.RawURLEncoding.EncodeToString([]byte("x")), "PS512", nil); err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
}

func TestVerifyRSARejectsBadHash(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := verifyRSA("input", []byte("bad-sig"), &key.PublicKey, crypto.SHA256); err == nil {
		t.Fatal("expected RSA verification failure")
	}
}

func TestHashFunc(t *testing.T) {
	t.Parallel()
	sizes := map[crypto.Hash]int{
		crypto.SHA256: 32,
		crypto.SHA384: 48,
		crypto.SHA512: 64,
		crypto.MD5:    32, // default branch falls back to SHA-256
	}
	for alg, size := range sizes {
		if got := hashFunc(alg).Size(); got != size {
			t.Fatalf("hashFunc(%v).Size() = %d, want %d", alg, got, size)
		}
	}
}

func TestSplitTokenAndIndexOf(t *testing.T) {
	t.Parallel()
	if got := splitToken("a.b.c"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("splitToken = %v", got)
	}
	if got := splitToken("no-dots"); got != nil {
		t.Fatalf("splitToken(no-dots) = %v, want nil", got)
	}
	if got := splitToken("only.one"); got != nil {
		t.Fatalf("splitToken(only.one) = %v, want nil", got)
	}
	if indexOf("abc", 'z', 0) != -1 {
		t.Fatal("indexOf missing char should be -1")
	}
}

func TestDecodeJWTSegment(t *testing.T) {
	t.Parallel()
	seg := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1"}`))
	m, err := decodeJWTSegment(seg)
	if err != nil {
		t.Fatalf("decodeJWTSegment: %v", err)
	}
	if m["sub"] != "user-1" {
		t.Fatalf("decoded sub = %v", m["sub"])
	}
	if _, err := decodeJWTSegment("!!!"); err == nil {
		t.Fatal("expected base64 decode error")
	}
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	if _, err := decodeJWTSegment(notJSON); err == nil {
		t.Fatal("expected JSON decode error")
	}
}
