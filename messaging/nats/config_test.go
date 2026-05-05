package nats

import (
	"reflect"
	"strings"
	"testing"
)

func TestConfigValidateAndRedactedURL(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", SubjectPrefix: "svc.events", QueueGroup: "workers", Token: "secret"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate nats config: %v", err)
	}
	redacted := cfg.RedactedURL()
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "user:") {
		t.Fatalf("redacted URL leaked credentials: %q", redacted)
	}
}

func TestConfigValidateRejectsURLCredentialsAndPlaintext(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "nats://user:secret@localhost:4222", AllowInsecureDev: true}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected URL credential validation error")
	}

	cfg = Config{URL: "nats://localhost:4222"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext validation error")
	}
	cfg.AllowInsecureDev = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("allow_insecure_dev should permit plaintext: %v", err)
	}
}

func TestConfigValidateRejectsConflictingAuth(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "tls://localhost:4222", Token: "token", Username: "user"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected conflicting auth validation error")
	}
}

func TestConfigDoesNotExposeCoreNameEnabled(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(Config{})
	for _, name := range []string{"Name", "Enabled"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("nats.Config exposes core field %s", name)
		}
	}
}
