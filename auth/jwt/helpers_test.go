package jwt_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth/jwt"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type testClaims struct {
	jwt.Registered
	UserID string `json:"user_id"`
}

func newClaims() *testClaims { return &testClaims{} }

func edKey(t testing.TB, id string) jwt.Key {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return jwt.Key{ID: id, Signer: private}
}

func verifyOnly(key jwt.Key, retireAt time.Time) jwt.Key {
	return jwt.Key{ID: key.ID, Public: key.Signer.Public(), RetireAt: retireAt}
}

func baseConfig() jwt.Config {
	return jwt.Config{
		Method:      jwt.EdDSA,
		Issuer:      "issuer",
		Audience:    []string{"svc-a", "svc-b"},
		Type:        "test+jwt",
		MaxLifetime: 30 * time.Second,
		Leeway:      2 * time.Second,
	}
}

func newService(t testing.TB, cfg jwt.Config, keys jwt.KeySet, clock util.Clock) *jwt.Service[*testClaims] {
	t.Helper()
	svc, err := jwt.NewService(cfg, keys, clock, newClaims)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func issue(t testing.TB, svc *jwt.Service[*testClaims], claims *testClaims) string {
	t.Helper()
	token, err := svc.GenerateToken(context.Background(), claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return token
}

func subject(aud ...string) *testClaims {
	c := &testClaims{UserID: "user-1"}
	c.Subject = "user-1"
	c.Audience = aud
	return c
}

func at(offset time.Duration) *gojwt.NumericDate { return gojwt.NewNumericDate(epoch.Add(offset)) }

func requireCode(t testing.TB, err error, code apperrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	var app *apperrors.AppError
	if !errors.As(err, &app) || app.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
