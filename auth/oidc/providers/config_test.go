package providers

import (
	"testing"
)

func TestDefaultScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   string
	}{
		{"google", NewGoogle(ProviderConfig{ClientID: "id"}).cfg.Scopes, "openid"},
		{"github", NewGitHub(ProviderConfig{ClientID: "id"}).cfg.Scopes, "read:user"},
		{"apple", NewApple(AppleConfig{ProviderConfig: ProviderConfig{ClientID: "id"}}).cfg.Scopes, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.scopes) == 0 || tt.scopes[0] != tt.want {
				t.Errorf("scopes = %v, want first=%q", tt.scopes, tt.want)
			}
		})
	}
}

func TestCustomScopesPreserved(t *testing.T) {
	custom := []string{"my:scope", "other:scope"}
	g := NewGoogle(ProviderConfig{ClientID: "id", Scopes: custom})

	if len(g.cfg.Scopes) != 2 || g.cfg.Scopes[0] != "my:scope" {
		t.Errorf("custom scopes overwritten: got %v, want %v", g.cfg.Scopes, custom)
	}
}

func TestWithDefaultScopes(t *testing.T) {
	// Empty scopes → defaults applied
	cfg := WithDefaultScopes(ProviderConfig{ClientID: "id"}, "a", "b")
	if len(cfg.Scopes) != 2 || cfg.Scopes[0] != "a" {
		t.Errorf("expected defaults [a b], got %v", cfg.Scopes)
	}

	// Non-empty scopes → preserved
	cfg = WithDefaultScopes(ProviderConfig{ClientID: "id", Scopes: []string{"x"}}, "a", "b")
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "x" {
		t.Errorf("expected custom [x], got %v", cfg.Scopes)
	}
}
