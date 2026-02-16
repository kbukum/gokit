package kafka

import (
	"fmt"
	"time"
)

// Config holds Kafka connection and behavior configuration.
type Config struct {
	// Enabled controls whether the Kafka component is active.
	Enabled bool `mapstructure:"enabled"`

	// Brokers is the list of Kafka broker addresses.
	Brokers []string `mapstructure:"brokers"`

	// GroupID is the consumer group identifier.
	GroupID string `mapstructure:"group_id"`

	// Topics is the list of topics to consume from.
	Topics []string `mapstructure:"topics"`

	// TLS
	EnableTLS     bool   `mapstructure:"enable_tls"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify"`
	TLSCAFile     string `mapstructure:"tls_ca_file"`
	TLSCertFile   string `mapstructure:"tls_cert_file"`
	TLSKeyFile    string `mapstructure:"tls_key_file"`

	// SASL
	EnableSASL    bool   `mapstructure:"enable_sasl"`
	SASLMechanism string `mapstructure:"sasl_mechanism"` // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Username      string `mapstructure:"username"`
	Password      string `mapstructure:"password"`

	// Producer settings
	Compression  string `mapstructure:"compression"` // none, gzip, snappy, lz4, zstd
	Retries      int    `mapstructure:"retries"`
	BatchSize    int    `mapstructure:"batch_size"`
	BatchTimeout string `mapstructure:"batch_timeout"`
	WriteTimeout string `mapstructure:"write_timeout"`
	ReadTimeout  string `mapstructure:"read_timeout"`
	RequiredAcks int    `mapstructure:"required_acks"`

	// Consumer settings
	SessionTimeout    string `mapstructure:"session_timeout"`
	HeartbeatInterval string `mapstructure:"heartbeat_interval"`
	RebalanceTimeout  string `mapstructure:"rebalance_timeout"`

	// Connection settings
	DialTimeout string `mapstructure:"dial_timeout"`
	IdleTimeout string `mapstructure:"idle_timeout"`
	MetadataTTL string `mapstructure:"metadata_ttl"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if len(c.Brokers) == 0 {
		c.Brokers = []string{"localhost:9092"}
	}
	if c.Compression == "" {
		c.Compression = "snappy"
	}
	if c.Retries <= 0 {
		c.Retries = 3
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.BatchTimeout == "" {
		c.BatchTimeout = "1s"
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = "10s"
	}
	if c.ReadTimeout == "" {
		c.ReadTimeout = "10s"
	}
	if c.RequiredAcks <= 0 {
		c.RequiredAcks = -1 // all replicas
	}
	if c.SessionTimeout == "" {
		c.SessionTimeout = "30s"
	}
	if c.HeartbeatInterval == "" {
		c.HeartbeatInterval = "3s"
	}
	if c.RebalanceTimeout == "" {
		c.RebalanceTimeout = "30s"
	}
	if c.DialTimeout == "" {
		c.DialTimeout = "10s"
	}
	if c.IdleTimeout == "" {
		c.IdleTimeout = "30s"
	}
	if c.MetadataTTL == "" {
		c.MetadataTTL = "6s"
	}
	if c.SASLMechanism == "" && c.EnableSASL {
		c.SASLMechanism = "PLAIN"
	}
}

// Validate checks that required fields are present and parseable.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}
	for _, d := range []struct {
		name, val string
	}{
		{"batch_timeout", c.BatchTimeout},
		{"write_timeout", c.WriteTimeout},
		{"read_timeout", c.ReadTimeout},
		{"session_timeout", c.SessionTimeout},
		{"heartbeat_interval", c.HeartbeatInterval},
		{"rebalance_timeout", c.RebalanceTimeout},
		{"dial_timeout", c.DialTimeout},
		{"idle_timeout", c.IdleTimeout},
		{"metadata_ttl", c.MetadataTTL},
	} {
		if _, err := time.ParseDuration(d.val); err != nil {
			return fmt.Errorf("invalid %s %q: %w", d.name, d.val, err)
		}
	}
	if c.EnableSASL {
		switch c.SASLMechanism {
		case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		default:
			return fmt.Errorf("unsupported SASL mechanism: %s", c.SASLMechanism)
		}
		if c.Username == "" {
			return fmt.Errorf("SASL username is required")
		}
	}
	if c.Retries <= 0 {
		return fmt.Errorf("retries must be > 0")
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be > 0")
	}
	return nil
}

// ParseDuration parses a duration string, returning zero on empty input.
func ParseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}
