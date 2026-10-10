package jwt_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth/jwt"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func header(t *testing.T, token string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var h map[string]any
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	return h
}

func TestGenerateTokenSetsProfileAndRoundTrips(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(epoch)
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, clock)

	token := issue(t, svc, subject("svc-a"))
	h := header(t, token)
	if h["kid"] != "k1" || h["typ"] != "test+jwt" || h["alg"] != "EdDSA" {
		t.Fatalf("unexpected header: %v", h)
	}

	got, err := svc.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got.UserID != "user-1" || got.Issuer != "issuer" || len(got.Audience) != 1 || got.Audience[0] != "svc-a" {
		t.Fatalf("unexpected claims: %+v", got)
	}
	if !got.IssuedAt.Equal(epoch) || !got.NotBefore.Equal(epoch) || !got.ExpiresAt.Equal(epoch.Add(30*time.Second)) {
		t.Fatalf("unexpected times: iat=%v nbf=%v exp=%v", got.IssuedAt, got.NotBefore, got.ExpiresAt)
	}
}

func TestGenerateRejectsOversizedToken(t *testing.T) {
	t.Parallel()
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch))
	claims := subject("svc-a")
	claims.UserID = strings.Repeat("x", 8<<10)
	token, err := svc.GenerateToken(context.Background(), claims)
	if err == nil || token != "" {
		t.Fatal("issuance returned a token outside its validation limit")
	}
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
}

func TestValidateTokenAcceptsEveryConfiguredAudience(t *testing.T) {
	t.Parallel()
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch))
	for _, aud := range []string{"svc-a", "svc-b"} {
		if _, err := svc.ValidateToken(context.Background(), issue(t, svc, subject(aud))); err != nil {
			t.Fatalf("audience %s rejected: %v", aud, err)
		}
	}
}

func TestExpiryHonorsExplicitLeeway(t *testing.T) {
	t.Parallel()
	for _, leeway := range []time.Duration{0, 2 * time.Second} {
		cfg := baseConfig()
		cfg.Leeway = leeway
		clock := util.NewFakeClock(epoch)
		svc := newService(t, cfg, jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, clock)
		token := issue(t, svc, subject("svc-a"))

		clock.Set(epoch.Add(30*time.Second + leeway - time.Second))
		if _, err := svc.ValidateToken(context.Background(), token); err != nil {
			t.Fatalf("leeway %s: rejected inside window: %v", leeway, err)
		}
		clock.Set(epoch.Add(30*time.Second + leeway))
		_, err := svc.ValidateToken(context.Background(), token)
		requireCode(t, err, apperrors.ErrCodeTokenExpired)
	}
}

func TestNotBeforeHonorsLeeway(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(epoch)
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, clock)
	claims := subject("svc-a")
	claims.NotBefore = at(10 * time.Second)
	token := issue(t, svc, claims)

	clock.Set(epoch.Add(7 * time.Second))
	_, err := svc.ValidateToken(context.Background(), token)
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
	clock.Set(epoch.Add(8 * time.Second))
	if _, err := svc.ValidateToken(context.Background(), token); err != nil {
		t.Fatalf("rejected at nbf-leeway: %v", err)
	}
}

func TestIssuedInFutureRejected(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(epoch)
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, clock)
	claims := subject("svc-a")
	claims.IssuedAt = at(5 * time.Second)
	token := issue(t, svc, claims)
	_, err := svc.ValidateToken(context.Background(), token)
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
}

func TestLifetimeAndOrdering(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	keys := jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}
	clock := util.NewFakeClock(epoch)
	strict := newService(t, baseConfig(), keys, clock)
	lenientCfg := baseConfig()
	lenientCfg.MaxLifetime = time.Hour
	lenient := newService(t, lenientCfg, keys, clock)

	cases := map[string]func(*testClaims){
		"exp-iat over maximum": func(c *testClaims) { c.ExpiresAt = at(31 * time.Second) },
		"nbf before iat":       func(c *testClaims) { c.IssuedAt = at(0); c.NotBefore = at(-time.Second) },
		"exp at nbf":           func(c *testClaims) { c.NotBefore = at(time.Second); c.ExpiresAt = at(time.Second) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			claims := subject("svc-a")
			mutate(claims)
			_, err := strict.GenerateToken(context.Background(), claims)
			requireCode(t, err, apperrors.ErrCodeInvalidInput)

			crafted := subject("svc-a")
			mutate(crafted)
			if name == "exp-iat over maximum" {
				token := issue(t, lenient, crafted)
				_, verr := strict.ValidateToken(context.Background(), token)
				requireCode(t, verr, apperrors.ErrCodeInvalidToken)
			}
		})
	}
	claims := subject("svc-a")
	claims.ExpiresAt = at(30 * time.Second)
	if _, err := strict.ValidateToken(context.Background(), issue(t, strict, claims)); err != nil {
		t.Fatalf("exact maximum lifetime rejected: %v", err)
	}
}

func TestBadlyOrderedTokenFromAnotherIssuerRejected(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, util.NewFakeClock(epoch))
	for name, claims := range map[string]gojwt.RegisteredClaims{
		"nbf before iat": {Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(-time.Second), ExpiresAt: at(10 * time.Second)},
		"missing nbf":    {Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), ExpiresAt: at(10 * time.Second)},
		"missing iat":    {Issuer: "issuer", Audience: []string{"svc-a"}, NotBefore: at(0), ExpiresAt: at(10 * time.Second)},
		"missing exp":    {Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0)},
		"wrong issuer":   {Issuer: "other", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)},
		"foreign aud":    {Issuer: "issuer", Audience: []string{"svc-z"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)},
		"missing aud":    {Issuer: "issuer", IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)},
	} {
		token := signRaw(t, key, map[string]any{"kid": "k1", "typ": "test+jwt"}, claims)
		_, err := svc.ValidateToken(context.Background(), token)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func signRaw(t *testing.T, key jwt.Key, headers map[string]any, claims gojwt.Claims) string {
	t.Helper()
	token := gojwt.NewWithClaims(gojwt.SigningMethodEdDSA, claims)
	for k, v := range headers {
		token.Header[k] = v
	}
	if _, ok := headers["typ"]; !ok {
		delete(token.Header, "typ")
	}
	signed, err := token.SignedString(key.Signer)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func validRaw() gojwt.RegisteredClaims {
	return gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)}
}

func TestTypeHeaderIsRequired(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, util.NewFakeClock(epoch))
	for typ, accepted := range map[string]bool{"": false, "JWT": false, "other+jwt": false, "test+jwt": true, "TEST+JWT": true, "application/test+jwt": true} {
		headers := map[string]any{"kid": "k1"}
		if typ != "" {
			headers["typ"] = typ
		}
		_, err := svc.ValidateToken(context.Background(), signRaw(t, key, headers, validRaw()))
		if accepted != (err == nil) {
			t.Fatalf("typ %q: accepted=%v err=%v", typ, accepted, err)
		}
	}
}

func TestKeyIDSelectsVerificationKey(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	other := edKey(t, "k2")
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, util.NewFakeClock(epoch))
	cases := map[string]string{
		"missing kid":         signRaw(t, key, map[string]any{"typ": "test+jwt"}, validRaw()),
		"unknown kid":         signRaw(t, key, map[string]any{"typ": "test+jwt", "kid": "k9"}, validRaw()),
		"numeric kid":         signRaw(t, key, map[string]any{"typ": "test+jwt", "kid": 1}, validRaw()),
		"kid of another key":  signRaw(t, other, map[string]any{"typ": "test+jwt", "kid": "k1"}, validRaw()),
		"non-string typ type": signRaw(t, key, map[string]any{"typ": 7, "kid": "k1"}, validRaw()),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.ValidateToken(context.Background(), token)
			requireCode(t, err, apperrors.ErrCodeInvalidToken)
		})
	}
}

func TestRotationOverlapEndsAtRetirement(t *testing.T) {
	t.Parallel()
	oldKey, newKey := edKey(t, "old"), edKey(t, "new")
	cfg := baseConfig()
	cfg.MaxLifetime = 10 * time.Minute
	clock := util.NewFakeClock(epoch)
	before := newService(t, cfg, jwt.KeySet{SigningKeyID: "old", Keys: []jwt.Key{oldKey}}, clock)
	token := issue(t, before, subject("svc-a"))

	after := newService(t, cfg, jwt.KeySet{SigningKeyID: "new", Keys: []jwt.Key{newKey, verifyOnly(oldKey, epoch.Add(time.Minute))}}, clock)
	if h := header(t, issue(t, after, subject("svc-a"))); h["kid"] != "new" {
		t.Fatalf("rotated service signs with %v", h["kid"])
	}
	clock.Set(epoch.Add(time.Minute - time.Second))
	if _, err := after.ValidateToken(context.Background(), token); err != nil {
		t.Fatalf("old key rejected inside overlap: %v", err)
	}
	clock.Set(epoch.Add(time.Minute))
	_, err := after.ValidateToken(context.Background(), token)
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
}

func TestSingleAudience(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	keys := jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}
	clock := util.NewFakeClock(epoch)
	cfg := baseConfig()
	cfg.SingleAudience = true
	single := newService(t, cfg, keys, clock)
	multi := newService(t, baseConfig(), keys, clock)

	_, err := single.GenerateToken(context.Background(), subject("svc-a", "svc-b"))
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
	_, err = single.ValidateToken(context.Background(), issue(t, multi, subject("svc-a", "svc-b")))
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
	if _, err := single.ValidateToken(context.Background(), issue(t, single, subject("svc-b"))); err != nil {
		t.Fatalf("single audience rejected: %v", err)
	}
}

func TestIssuanceDefaultsAndRejectsForeignProfile(t *testing.T) {
	t.Parallel()
	keys := jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}
	clock := util.NewFakeClock(epoch)
	multi := newService(t, baseConfig(), keys, clock)
	_, err := multi.GenerateToken(context.Background(), subject())
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
	_, err = multi.GenerateToken(context.Background(), subject("svc-z"))
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
	foreign := subject("svc-a")
	foreign.Issuer = "other"
	_, err = multi.GenerateToken(context.Background(), foreign)
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
	_, err = multi.GenerateToken(context.Background(), nil)
	requireCode(t, err, apperrors.ErrCodeInvalidInput)

	cfg := baseConfig()
	cfg.Audience = []string{"only"}
	one := newService(t, cfg, keys, clock)
	got, err := one.ValidateToken(context.Background(), issue(t, one, subject()))
	if err != nil || len(got.Audience) != 1 || got.Audience[0] != "only" {
		t.Fatalf("default audience: %+v %v", got, err)
	}
}

func TestVerifyOnlyServiceCannotSign(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	clock := util.NewFakeClock(epoch)
	signer := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, clock)
	verifier := newService(t, baseConfig(), jwt.KeySet{Keys: []jwt.Key{{ID: "k1", Public: key.Signer.Public()}}}, clock)
	if _, err := verifier.ValidateToken(context.Background(), issue(t, signer, subject("svc-a"))); err != nil {
		t.Fatalf("verify-only rejected: %v", err)
	}
	_, err := verifier.GenerateToken(context.Background(), subject("svc-a"))
	requireCode(t, err, apperrors.ErrCodeInvalidInput)
}

func TestCanceledContext(t *testing.T) {
	t.Parallel()
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch))
	token := issue(t, svc, subject("svc-a"))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	for ctx, code := range map[context.Context]apperrors.ErrorCode{canceled: apperrors.ErrCodeCanceled, expired: apperrors.ErrCodeTimeout} {
		_, err := svc.ValidateToken(ctx, token)
		requireCode(t, err, code)
		_, err = svc.GenerateToken(ctx, subject("svc-a"))
		requireCode(t, err, code)
	}
}

func TestServiceSatisfiesTokenContracts(t *testing.T) {
	t.Parallel()
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch))
	var _ interface {
		ValidateToken(context.Context, string) (*testClaims, error)
		GenerateToken(context.Context, *testClaims) (string, error)
	} = svc
	reg := &jwt.Registered{}
	reg.Audience = []string{"svc-a"}
	plain, err := jwt.NewService(baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{edKey(t, "k1")}}, util.NewFakeClock(epoch), func() *jwt.Registered { return &jwt.Registered{} })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.GenerateToken(context.Background(), reg); err != nil {
		t.Fatalf("bare registered claims: %v", err)
	}
}

// vetoClaims fails the application's own claim check.
type vetoClaims struct{ jwt.Registered }

func (*vetoClaims) Validate() error { return errors.New("application veto") }

func TestExpiredIsReportedOnlyWhenItIsTheSoleDefect(t *testing.T) {
	t.Parallel()
	key := edKey(t, "k1")
	clock := util.NewFakeClock(epoch.Add(time.Hour))
	svc := newService(t, baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, clock)
	header := map[string]any{"kid": "k1", "typ": "test+jwt"}
	for name, tc := range map[string]struct {
		claims gojwt.RegisteredClaims
		code   apperrors.ErrorCode
	}{
		"only expired":   {gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)}, apperrors.ErrCodeTokenExpired},
		"wrong issuer":   {gojwt.RegisteredClaims{Issuer: "other", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)}, apperrors.ErrCodeInvalidToken},
		"foreign aud":    {gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-z"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)}, apperrors.ErrCodeInvalidToken},
		"nbf before iat": {gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(-time.Second), ExpiresAt: at(10 * time.Second)}, apperrors.ErrCodeInvalidToken},
		"over lifetime":  {gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(time.Minute)}, apperrors.ErrCodeInvalidToken},
		"missing nbf":    {gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), ExpiresAt: at(10 * time.Second)}, apperrors.ErrCodeInvalidToken},
	} {
		_, err := svc.ValidateToken(context.Background(), signRaw(t, key, header, tc.claims))
		if got := apperrors.Normalize(err).Code; got != tc.code {
			t.Errorf("%s: got %v, want %v", name, got, tc.code)
		}
	}

	veto, err := jwt.NewService(baseConfig(), jwt.KeySet{SigningKeyID: "k1", Keys: []jwt.Key{key}}, clock, func() *vetoClaims { return &vetoClaims{} })
	if err != nil {
		t.Fatal(err)
	}
	_, err = veto.ValidateToken(context.Background(), signRaw(t, key, header, gojwt.RegisteredClaims{Issuer: "issuer", Audience: []string{"svc-a"}, IssuedAt: at(0), NotBefore: at(0), ExpiresAt: at(10 * time.Second)}))
	requireCode(t, err, apperrors.ErrCodeInvalidToken)
}
