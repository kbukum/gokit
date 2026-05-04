package kafka

import (
	"crypto/tls"
	"reflect"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/security"
)

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if len(cfg.Brokers) != 1 || cfg.Brokers[0] != "localhost:9092" {
		t.Errorf("Brokers = %v, want [localhost:9092]", cfg.Brokers)
	}
	if cfg.Compression != "snappy" {
		t.Errorf("Compression = %q, want snappy", cfg.Compression)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if cfg.BatchTimeout != "1s" {
		t.Errorf("BatchTimeout = %q, want 1s", cfg.BatchTimeout)
	}
	if cfg.RequiredAcks != -1 {
		t.Errorf("RequiredAcks = %d, want -1", cfg.RequiredAcks)
	}
	if cfg.SessionTimeout != "30s" {
		t.Errorf("SessionTimeout = %q, want 30s", cfg.SessionTimeout)
	}
	if cfg.HeartbeatInterval != "3s" {
		t.Errorf("HeartbeatInterval = %q, want 3s", cfg.HeartbeatInterval)
	}
	if cfg.RebalanceTimeout != "30s" {
		t.Errorf("RebalanceTimeout = %q, want 30s", cfg.RebalanceTimeout)
	}
	if cfg.DialTimeout != "10s" {
		t.Errorf("DialTimeout = %q, want 10s", cfg.DialTimeout)
	}
	if cfg.IdleTimeout != "30s" {
		t.Errorf("IdleTimeout = %q, want 30s", cfg.IdleTimeout)
	}
	if cfg.MetadataTTL != "6s" {
		t.Errorf("MetadataTTL = %q, want 6s", cfg.MetadataTTL)
	}
	if cfg.TLS == nil || cfg.TLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS default = %#v, want TLS 1.2 floor", cfg.TLS)
	}
}

func TestConfig_ApplyDefaults_AllowsExplicitInsecureDevPlaintext(t *testing.T) {
	cfg := Config{AllowInsecureDev: true}
	cfg.ApplyDefaults()

	if cfg.TLS != nil {
		t.Fatalf("TLS = %#v, want nil when allow_insecure_dev is explicit", cfg.TLS)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with allow_insecure_dev: %v", err)
	}
}

func TestConfigDoesNotExposeCorePolicyFields(t *testing.T) {
	typ := reflect.TypeOf(Config{})
	for _, name := range []string{"Name", "Enabled", "Retries", "RetryAttempts", "WriteTimeout", "ReadTimeout", "GroupID", "Topics", "DLQ", "MaxInFlight", "CommitStrategy", "DeliveryGuarantee"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("kafka.Config exposes core field %s", name)
		}
	}
}

func TestValidateCommonRejectsAdapterManagedDLQ(t *testing.T) {
	t.Parallel()

	cfg := messaging.Config{Backend: "kafka", DLQ: messaging.DLQPolicy{Enabled: true}}
	cfg.ApplyDefaults()
	if err := ValidateCommonProducer(cfg); err == nil {
		t.Fatal("expected producer DLQ validation error")
	}
	if err := ValidateCommonConsumer(cfg); err == nil {
		t.Fatal("expected consumer DLQ validation error")
	}
}

func TestConfig_ApplyDefaults_NoOverwrite(t *testing.T) {
	cfg := Config{
		Brokers:     []string{"broker1:9092", "broker2:9092"},
		TLS:         &security.TLSConfig{ServerName: "kafka.test"},
		Compression: "gzip",
		BatchSize:   200,
	}
	cfg.ApplyDefaults()

	if len(cfg.Brokers) != 2 {
		t.Errorf("Brokers should not be overwritten, got %v", cfg.Brokers)
	}
	if cfg.Compression != "gzip" {
		t.Errorf("Compression should not be overwritten, got %q", cfg.Compression)
	}
	if cfg.BatchSize != 200 {
		t.Errorf("BatchSize should not be overwritten, got %d", cfg.BatchSize)
	}
}

func TestConfig_ApplyDefaults_SASLDefaultMechanism(t *testing.T) {
	cfg := Config{EnableSASL: true}
	cfg.ApplyDefaults()
	if cfg.SASLMechanism != "PLAIN" {
		t.Errorf("SASLMechanism = %q, want PLAIN", cfg.SASLMechanism)
	}
}

func TestConfig_Validate_NoBrokers(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	cfg.Brokers = []string{}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with no brokers")
	}
}

func TestConfig_Validate_InvalidDuration(t *testing.T) {
	cfg := Config{
		Brokers:      []string{"localhost:9092"},
		TLS:          &security.TLSConfig{ServerName: "kafka.test"},
		BatchTimeout: "not-a-duration",
	}
	cfg.ApplyDefaults()
	cfg.BatchTimeout = "not-a-duration"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid duration")
	}
}

func TestConfig_Validate_UnsupportedSASL(t *testing.T) {
	cfg := Config{EnableSASL: true, SASLMechanism: "GSSAPI", Username: "u"}
	cfg.ApplyDefaults()
	cfg.SASLMechanism = "GSSAPI"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with unsupported SASL mechanism")
	}
}

func TestConfig_Validate_SASLNoUsername(t *testing.T) {
	cfg := Config{EnableSASL: true, SASLMechanism: "PLAIN"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with no SASL username")
	}
}

func TestConfig_Validate_InvalidBatchSize(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	cfg.BatchSize = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with batch_size=0")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{Brokers: []string{"localhost:9092"}, TLS: &security.TLSConfig{ServerName: "kafka.test"}}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass: %v", err)
	}
}

func TestConfig_Validate_ValidSASL(t *testing.T) {
	for _, mech := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		t.Run(mech, func(t *testing.T) {
			cfg := Config{
				EnableSASL:    true,
				SASLMechanism: mech,
				Username:      "user",
				Password:      "pass",
				TLS:           &security.TLSConfig{ServerName: "kafka.test"},
			}
			cfg.ApplyDefaults()
			cfg.SASLMechanism = mech
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() should pass for %s: %v", mech, err)
			}
		})
	}
}

func TestConfigValidateRejectsPlaintextWithoutDevOptIn(t *testing.T) {
	cfg := Config{TLS: nil, AllowInsecureDev: false}
	cfg.ApplyDefaults()
	cfg.TLS = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext validation error")
	}
	cfg.AllowInsecureDev = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("allow_insecure_dev should permit plaintext: %v", err)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"1s", time.Second},
		{"500ms", 500 * time.Millisecond},
		{"30s", 30 * time.Second},
		{"", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseDuration(tt.input); got != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
