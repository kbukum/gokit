package kafka

import (
	"testing"
	"time"
)

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{Enabled: true}
	cfg.ApplyDefaults()

	if len(cfg.Brokers) != 1 || cfg.Brokers[0] != "localhost:9092" {
		t.Errorf("Brokers = %v, want [localhost:9092]", cfg.Brokers)
	}
	if cfg.Compression != "snappy" {
		t.Errorf("Compression = %q, want snappy", cfg.Compression)
	}
	if cfg.Retries != 3 {
		t.Errorf("Retries = %d, want 3", cfg.Retries)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if cfg.BatchTimeout != "1s" {
		t.Errorf("BatchTimeout = %q, want 1s", cfg.BatchTimeout)
	}
	if cfg.WriteTimeout != "10s" {
		t.Errorf("WriteTimeout = %q, want 10s", cfg.WriteTimeout)
	}
	if cfg.ReadTimeout != "10s" {
		t.Errorf("ReadTimeout = %q, want 10s", cfg.ReadTimeout)
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
}

func TestConfig_ApplyDefaults_NoOverwrite(t *testing.T) {
	cfg := Config{
		Brokers:     []string{"broker1:9092", "broker2:9092"},
		Compression: "gzip",
		Retries:     5,
		BatchSize:   200,
	}
	cfg.ApplyDefaults()

	if len(cfg.Brokers) != 2 {
		t.Errorf("Brokers should not be overwritten, got %v", cfg.Brokers)
	}
	if cfg.Compression != "gzip" {
		t.Errorf("Compression should not be overwritten, got %q", cfg.Compression)
	}
	if cfg.Retries != 5 {
		t.Errorf("Retries should not be overwritten, got %d", cfg.Retries)
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

func TestConfig_Validate_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass for disabled config: %v", err)
	}
}

func TestConfig_Validate_NoBrokers(t *testing.T) {
	cfg := Config{Enabled: true, Brokers: []string{}}
	cfg.ApplyDefaults()
	cfg.Brokers = []string{} // clear after defaults
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with no brokers")
	}
}

func TestConfig_Validate_InvalidDuration(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		BatchTimeout: "not-a-duration",
	}
	cfg.ApplyDefaults()
	cfg.BatchTimeout = "not-a-duration"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid duration")
	}
}

func TestConfig_Validate_UnsupportedSASL(t *testing.T) {
	cfg := Config{Enabled: true, EnableSASL: true, SASLMechanism: "GSSAPI", Username: "u"}
	cfg.ApplyDefaults()
	cfg.SASLMechanism = "GSSAPI"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with unsupported SASL mechanism")
	}
}

func TestConfig_Validate_SASLNoUsername(t *testing.T) {
	cfg := Config{Enabled: true, EnableSASL: true, SASLMechanism: "PLAIN"}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with no SASL username")
	}
}

func TestConfig_Validate_InvalidRetries(t *testing.T) {
	cfg := Config{Enabled: true}
	cfg.ApplyDefaults()
	cfg.Retries = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with retries=0")
	}
}

func TestConfig_Validate_InvalidBatchSize(t *testing.T) {
	cfg := Config{Enabled: true}
	cfg.ApplyDefaults()
	cfg.BatchSize = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with batch_size=0")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Brokers: []string{"localhost:9092"},
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass: %v", err)
	}
}

func TestConfig_Validate_ValidSASL(t *testing.T) {
	for _, mech := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		t.Run(mech, func(t *testing.T) {
			cfg := Config{
				Enabled:       true,
				EnableSASL:    true,
				SASLMechanism: mech,
				Username:      "user",
				Password:      "pass",
			}
			cfg.ApplyDefaults()
			cfg.SASLMechanism = mech
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() should pass for %s: %v", mech, err)
			}
		})
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
