package jwt_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/jwt"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func TestKeySetValidation(t *testing.T) {
	t.Parallel()
	k1, k2 := edKey(t, "k1"), edKey(t, "k2")
	_, otherPrivate, _ := ed25519.GenerateKey(rand.Reader)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]jwt.KeySet{
		"empty":                      {},
		"unknown signing key":        {SigningKeyID: "k9", Keys: []jwt.Key{k1}},
		"duplicate ids":              {SigningKeyID: "k1", Keys: []jwt.Key{k1, {ID: "k1", Public: k2.Signer.Public(), RetireAt: epoch}}},
		"empty id":                   {Keys: []jwt.Key{{Public: k1.Signer.Public()}}},
		"id with space":              {Keys: []jwt.Key{{ID: "a b", Public: k1.Signer.Public()}}},
		"id too long":                {Keys: []jwt.Key{{ID: strings.Repeat("a", 129), Public: k1.Signer.Public()}}},
		"unbounded second key":       {SigningKeyID: "k1", Keys: []jwt.Key{k1, verifyOnly(k2, time.Time{})}},
		"retiring signing key":       {SigningKeyID: "k1", Keys: []jwt.Key{{ID: "k1", Signer: k1.Signer, RetireAt: epoch}}},
		"signing key without signer": {SigningKeyID: "k1", Keys: []jwt.Key{verifyOnly(k1, time.Time{})}},
		"no material":                {Keys: []jwt.Key{{ID: "k1"}}},
		"mismatched public":          {Keys: []jwt.Key{{ID: "k1", Signer: k1.Signer, Public: otherPrivate.Public()}}},
		"wrong key type":             {Keys: []jwt.Key{{ID: "k1", Signer: rsaKey}}},
		"secret for asymmetric":      {Keys: []jwt.Key{{ID: "k1", Secret: []byte(strings.Repeat("s", 32))}}},
	}
	for i := 0; i < 9; i++ {
		cases["too many keys"] = jwt.KeySet{Keys: append(cases["too many keys"].Keys, verifyOnly(edKey(t, "k"+strings.Repeat("x", i+1)), epoch))}
	}
	for name, set := range cases {
		_, err := jwt.NewService(baseConfig(), set, util.NewFakeClock(epoch), newClaims)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		requireCode(t, err, apperrors.ErrCodeInvalidInput)
	}
}

func TestKeyMaterialRequirements(t *testing.T) {
	t.Parallel()
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		method jwt.SigningMethod
		key    jwt.Key
	}{
		{jwt.RS256, jwt.Key{ID: "k1", Signer: small}},
		{jwt.ES256, jwt.Key{ID: "k1", Signer: p384}},
		{jwt.HS256, jwt.Key{ID: "k1", Secret: []byte("short")}},
		{jwt.HS256, jwt.Key{ID: "k1", Signer: p384}},
		{jwt.EdDSA, jwt.Key{ID: "k1", Public: ed25519.PublicKey([]byte("short"))}},
	}
	for _, tc := range cases {
		cfg := baseConfig()
		cfg.Method = tc.method
		cfg.AllowSymmetricHMAC = tc.method == jwt.HS256
		if _, err := jwt.NewService(cfg, jwt.KeySet{Keys: []jwt.Key{tc.key}}, util.NewFakeClock(epoch), newClaims); err == nil {
			t.Errorf("%s: weak or mismatched key accepted", tc.method)
		}
	}
}

func TestParseKey(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	signing, err := jwt.ParseKey("k1", privatePEM)
	if err != nil || signing.Signer == nil || signing.ID != "k1" {
		t.Fatalf("private PEM: %+v %v", signing, err)
	}
	verifying, err := jwt.ParseKey("k1", publicPEM)
	if err != nil || verifying.Public == nil || verifying.Signer != nil {
		t.Fatalf("public PEM: %+v %v", verifying, err)
	}
	clock := util.NewFakeClock(epoch)
	signer := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{signing}}, clock)
	verifier := newService(t, baseConfig(), jwt.KeySet{Keys: []jwt.Key{verifying}}, clock)
	if _, err := verifier.ValidateToken(t.Context(), issue(t, signer, subject("svc-a"))); err != nil {
		t.Fatalf("parsed keys do not interoperate: %v", err)
	}

	for name, input := range map[string][]byte{
		"empty":         nil,
		"not pem":       []byte("garbage"),
		"wrong type":    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: publicDER}),
		"bad der":       pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")}),
		"bad public":    pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("x")}),
		"two blocks":    append(append([]byte{}, publicPEM...), publicPEM...),
		"trailing junk": append(append([]byte{}, publicPEM...), []byte("junk")...),
	} {
		if _, err := jwt.ParseKey("k1", input); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("%s: want typed invalid input, got %v", name, err)
		}
	}
}
