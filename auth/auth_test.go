package auth

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/auth/jwt"
	"github.com/kbukum/gokit/auth/oidc"
	"github.com/kbukum/gokit/auth/password"
)

func TestTokenValidatorAndGeneratorAdapters(t *testing.T) {
	t.Parallel()

	validator := NewValidator(func(token string) (any, error) {
		return "claims:" + token, nil
	})
	claims, err := validator.ValidateToken("abc")
	if err != nil || claims.(string) != "claims:abc" {
		t.Fatalf("unexpected validator result: %v %v", claims, err)
	}

	generator := TokenGeneratorFunc(func(claims any) (string, error) {
		return claims.(string) + "-token", nil
	})
	token, err := generator.GenerateToken("access")
	if err != nil || token != "access-token" {
		t.Fatalf("unexpected generator result: %q %v", token, err)
	}
}

func TestRegistry_DefaultAndSetDefault(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	first := TokenValidatorFunc(func(token string) (any, error) { return token, nil })
	second := TokenValidatorFunc(func(token string) (any, error) { return token + "-2", nil })

	if err := reg.Register("first", first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	if err := reg.Register("second", second); err != nil {
		t.Fatalf("Register second: %v", err)
	}
	if got, ok := reg.Get("first"); !ok {
		t.Fatal("expected first validator to be retrievable")
	} else if value, err := got.ValidateToken("t"); err != nil || value.(string) != "t" {
		t.Fatalf("unexpected Get result: %v %v", value, err)
	}
	names := reg.Names()
	if len(names) != 2 || names[0] != "first" || names[1] != "second" {
		t.Fatalf("unexpected names: %v", names)
	}

	def, ok := reg.Default()
	if !ok {
		t.Fatal("expected default validator")
	}
	got, err := def.ValidateToken("t")
	if err != nil || got.(string) != "t" {
		t.Fatalf("unexpected default validator result: %v %v", got, err)
	}

	if setErr := reg.SetDefault("second"); setErr != nil {
		t.Fatalf("SetDefault: %v", setErr)
	}
	def, _ = reg.Default()
	got, err = def.ValidateToken("t")
	if err != nil || got.(string) != "t-2" {
		t.Fatalf("unexpected updated default validator result: %v %v", got, err)
	}
}

func TestRegistry_SetDefaultMissing(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if err := reg.SetDefault("missing"); err == nil {
		t.Fatal("expected missing validator error")
	}
}

func TestConfig_ApplyDefaultsValidateDescribe(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Enabled: true,
		JWT: &jwt.Config{
			Method:             jwt.HS256,
			AllowSymmetricHMAC: true,
			Secret:             "12345678901234567890123456789012",
			Issuer:             "issuer",
			Audience:           []string{"aud"},
		},
		Password: &password.Config{},
		OIDC: &oidc.Config{
			Enabled:     true,
			Issuer:      "https://issuer.example.com",
			ClientID:    "client-id",
			RedirectURL: "https://app.example.com/callback",
		},
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := cfg.Describe(); got == "" || got == "disabled" {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestConfig_ValidateWrappedError(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Enabled: true,
		JWT: &jwt.Config{
			Method:   jwt.RS256,
			Issuer:   "issuer",
			Audience: []string{"aud"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wrapped validation error")
	}
}

func TestTokenValidatorFunc_ErrorPropagation(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	validator := TokenValidatorFunc(func(token string) (any, error) { return nil, want })
	if _, err := validator.ValidateToken("x"); !errors.Is(err, want) {
		t.Fatalf("expected propagated error, got %v", err)
	}
}
