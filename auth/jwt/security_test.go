package jwt_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth/jwt"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func compact(header, payload, signature string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(header)) + "." + enc([]byte(payload)) + "." + enc([]byte(signature))
}

func hmacService(t testing.TB, secret string) *jwt.Service[*testClaims] {
	cfg := baseConfig()
	cfg.Method, cfg.AllowSymmetricHMAC = jwt.HS256, true
	return newService(t, cfg, jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{{ID: "k1", Secret: []byte(secret)}}}, util.NewFakeClock(epoch))
}

func TestSecurityDifferentHMACSecretRejected(t *testing.T) {
	t.Parallel()
	a := hmacService(t, strings.Repeat("a", 32))
	b := hmacService(t, strings.Repeat("b", 32))
	_, err := b.ValidateToken(context.Background(), issue(t, a, subject("svc-a")))
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
}

func TestSecurityNoneAndConfusedAlgorithmsRejected(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, util.NewFakeClock(epoch))
	payload := `{"iss":"issuer","aud":"svc-a","iat":1767268800,"nbf":1767268800,"exp":1767268810}`
	none := compact(`{"alg":"none","typ":"test+jwt","kid":"k1"}`, payload, "")
	_, err := svc.ValidateToken(context.Background(), none)
	requireCode(t, err, apperrors.ErrCodeInvalidToken)

	// An HS256 token keyed with the public key bytes must not verify against an EdDSA key set.
	confused := gojwt.NewWithClaims(gojwt.SigningMethodHS256, validRaw())
	confused.Header["kid"], confused.Header["typ"] = "k1", "test+jwt"
	public, ok := key.Signer.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatal("unexpected public key type")
	}
	signed, err := confused.SignedString([]byte(public))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ValidateToken(context.Background(), signed)
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
}

func TestSecurityMalformedAndOversizedTokens(t *testing.T) {
	t.Parallel()
	svc := hmacService(t, strings.Repeat("s", 32))
	for _, token := range []string{"", "aaa", "aaa.bbb", "こんにちは.世界.テスト", "   ", strings.Repeat("a", 100_000), "a\x00b.c\x00d.e\x00f"} {
		_, err := svc.ValidateToken(context.Background(), token)
		requireCode(t, err, apperrors.ErrCodeInvalidToken)
	}
}

func TestSecurityErrorsDoNotLeakMaterial(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("q", 32)
	svc := hmacService(t, secret)
	other := hmacService(t, strings.Repeat("r", 32))
	token := issue(t, other, subject("svc-a"))
	_, err := svc.ValidateToken(context.Background(), token)
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), token) {
		t.Fatalf("error leaks material: %v", err)
	}
}

func TestSecurityConcurrentValidation(t *testing.T) {
	t.Parallel()
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch))
	token := issue(t, svc, subject("svc-a"))
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Go(func() {
			if _, err := svc.ValidateToken(context.Background(), token); err != nil {
				errs <- err
			}
			if _, err := svc.GenerateToken(context.Background(), subject("svc-b")); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func FuzzValidateToken(f *testing.F) {
	for _, seed := range []string{
		"", ".", "..", "a.b.c",
		compact(`{"alg":"none","typ":"test+jwt","kid":"k1"}`, `{"sub":"1"}`, ""),
		compact(`{"alg":"HS256","typ":"test+jwt","kid":"k1"}`, `{"sub":"1"}`, "bad"),
		compact(`{"alg":"EdDSA","typ":"test+jwt","kid":"k1"}`, `{"sub":"1"}`, "bad"),
		"\x00\x00\x00", string(make([]byte, 64<<10)),
	} {
		f.Add(seed)
	}
	svc := newService(f, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(f, "k1")}}, util.NewFakeClock(epoch))
	f.Fuzz(func(t *testing.T, token string) {
		if _, err := svc.ValidateToken(context.Background(), token); err == nil {
			t.Fatalf("fuzzed token accepted: %q", token)
		}
	})
}
