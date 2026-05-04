package rabbitmq

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestConfigDefaultURLDoesNotEmbedCredentials(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.URL != defaultURL {
		t.Fatalf("default URL = %q, want %q", cfg.URL, defaultURL)
	}
	if strings.Contains(cfg.URL, "@") {
		t.Fatalf("default URL contains credentials: %q", cfg.URL)
	}
}

func TestRedactedErrorHidesAMQPCredentials(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("dial amqps://service:secret@localhost:5672/ failed")
	err := redactError("rabbitmq connect", sentinel)

	if !errors.Is(err, sentinel) {
		t.Fatal("redacted error should unwrap original error")
	}
	text := err.Error()
	if strings.Contains(text, "service:secret") {
		t.Fatalf("error leaked credentials: %q", text)
	}
	if !strings.Contains(text, "amqps://[redacted]@localhost:5672/") {
		t.Fatalf("error = %q, want redacted URL", text)
	}
}

func TestConfigValidateRejectsInvalidExchangeType(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "amqps://localhost:5671/", ExchangeType: "invalid"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid exchange type error")
	}
}

func TestConfigRedactedURL(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "amqps://user:secret@localhost:5671/"}
	if redacted := cfg.RedactedURL(); strings.Contains(redacted, "secret") || strings.Contains(redacted, "user:") {
		t.Fatalf("redacted URL leaked credentials: %q", redacted)
	}

	cfg = Config{URL: "amqps://localhost:5671/", Username: "service", Password: "secret"}
	if redacted := cfg.RedactedURL(); strings.Contains(redacted, "secret") || strings.Contains(redacted, "service:") {
		t.Fatalf("redacted URL leaked configured credentials: %q", redacted)
	}
}

func TestConfigValidateRejectsURLCredentialsAndPlaintext(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "amqp://user:secret@localhost:5672/", AllowInsecureDev: true}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected URL credential validation error")
	}

	cfg = Config{URL: "amqp://localhost:5672/"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext validation error")
	}
	cfg.AllowInsecureDev = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("allow_insecure_dev should permit plaintext: %v", err)
	}

	cfg = Config{URL: "amqps://localhost:5671/", Password: "secret"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected username-required validation error")
	}
}

func TestConfigDoesNotExposeCoreNameEnabled(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(Config{})
	for _, name := range []string{"Name", "Enabled"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("rabbitmq.Config exposes core field %s", name)
		}
	}
}
