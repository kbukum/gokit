package qdrant

import "testing"

func TestConfigValidateRejectsSensitiveURLForms(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "ftp://qdrant.example.test", "https://user:pass@qdrant.example.test", "https://qdrant.example.test?api_key=secret", "https://qdrant.example.test#secret"} {
		cfg := Config{URL: raw}
		cfg.ApplyDefaults()
		if raw == "" {
			cfg.URL = raw
		}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", raw)
		}
	}
}

func FuzzValidateEndpoint(f *testing.F) {
	for _, seed := range []string{"http://localhost:6333", "https://qdrant.example.test", "ftp://bad", "https://host?x=y"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_ = validateEndpoint(raw)
	})
}

func TestValidateEndpointRejectsCredentialsAndEmptyHost(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"https://user:pass@qdrant.example.test", "http:///only-path", "http://[::1"} {
		if err := validateEndpoint(raw); err == nil {
			t.Fatalf("validateEndpoint(%q) should fail", raw)
		}
	}
}

func TestValidateRejectsNonPositiveTimeout(t *testing.T) {
	t.Parallel()
	cfg := Config{URL: "https://qdrant.example.test", Metric: "cosine", Timeout: 0}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-positive timeout to be rejected")
	}
}
