package jwt_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/jwt"
	"github.com/kbukum/gokit/util"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*jwt.Config){
		"missing issuer":        func(c *jwt.Config) { c.Issuer = "" },
		"missing audience":      func(c *jwt.Config) { c.Audience = nil },
		"empty audience":        func(c *jwt.Config) { c.Audience = []string{""} },
		"duplicate audience":    func(c *jwt.Config) { c.Audience = []string{"a", "a"} },
		"missing type":          func(c *jwt.Config) { c.Type = "" },
		"type with space":       func(c *jwt.Config) { c.Type = "a b" },
		"missing max lifetime":  func(c *jwt.Config) { c.MaxLifetime = 0 },
		"negative leeway":       func(c *jwt.Config) { c.Leeway = -time.Second },
		"leeway over a minute":  func(c *jwt.Config) { c.Leeway = time.Minute + time.Second },
		"unsupported method":    func(c *jwt.Config) { c.Method = "PS512" },
		"hmac without opt-in":   func(c *jwt.Config) { c.Method = jwt.HS256 },
		"single audience empty": func(c *jwt.Config) { c.SingleAudience = true; c.Audience = nil },
	}
	for name, mutate := range cases {
		cfg := baseConfig()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	cfg := baseConfig()
	cfg.Leeway = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero leeway rejected: %v", err)
	}
}

func TestApplyDefaultsKeepsExplicitZeroLeeway(t *testing.T) {
	t.Parallel()
	cfg := jwt.Config{}
	cfg.ApplyDefaults()
	if cfg.Method != jwt.RS256 || cfg.Leeway != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestNewServiceRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	keys := jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}
	clock := util.NewFakeClock(epoch)
	if _, err := jwt.NewService(baseConfig(), keys, nil, newClaims); err == nil {
		t.Fatal("nil clock accepted")
	}
	if _, err := jwt.NewService[*testClaims](baseConfig(), keys, clock, nil); err == nil {
		t.Fatal("nil constructor accepted")
	}
	bad := baseConfig()
	bad.Type = ""
	if _, err := jwt.NewService(bad, keys, clock, newClaims); err == nil {
		t.Fatal("invalid config accepted")
	}
}

func TestAlgorithmsRoundTrip(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		method jwt.SigningMethod
		key    jwt.Key
	}{
		{jwt.RS256, jwt.Key{ID: "k1", Signer: rsaKey}},
		{jwt.ES256, jwt.Key{ID: "k1", Signer: ecKey}},
		{jwt.EdDSA, edKey(t, "k1")},
		{jwt.HS256, jwt.Key{ID: "k1", Secret: []byte(strings.Repeat("s", 32))}},
	}
	for _, tc := range cases {
		cfg := baseConfig()
		cfg.Method = tc.method
		cfg.AllowSymmetricHMAC = tc.method == jwt.HS256
		svc := newService(t, cfg, jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{tc.key}}, util.NewFakeClock(epoch))
		if _, err := svc.ValidateToken(t.Context(), issue(t, svc, subject("svc-a"))); err != nil {
			t.Fatalf("%s: %v", tc.method, err)
		}
	}
}
