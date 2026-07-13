package gcs

import "testing"

func TestConfigValidateRequiresBucket(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing bucket to fail")
	}
}

func TestConfigValidateRejectsConflictingCredentials(t *testing.T) {
	t.Parallel()
	cfg := Config{Bucket: "objects", CredentialsFile: "creds.json", CredentialsJSON: []byte("{}")}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflicting credentials to fail")
	}
}

func TestConfigValidateRequiresCompleteSigningPair(t *testing.T) {
	t.Parallel()
	cfg := Config{Bucket: "objects", GoogleAccessID: "svc@example.test"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected incomplete signing pair to fail")
	}
}
