package kafka

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/security"
)

// Config holds Kafka-specific connection, protocol, and batching settings.
type Config struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string `yaml:"brokers" mapstructure:"brokers"`

	// Resolve is the discovery service name for Kafka brokers. Empty = use static Brokers. Set = resolve from discovery provider.
	Resolve string `yaml:"resolve" mapstructure:"resolve"`

	// TLS configures TLS settings. Defaults to a TLS 1.2 floor unless AllowInsecureDev is explicitly enabled.
	TLS *security.TLSConfig `yaml:"tls" mapstructure:"tls"`

	// AllowInsecureDev permits plaintext connections for local development only.
	AllowInsecureDev bool `yaml:"allow_insecure_dev" mapstructure:"allow_insecure_dev"`

	// SASL
	EnableSASL    bool   `yaml:"enable_sasl" mapstructure:"enable_sasl"`
	SASLMechanism string `yaml:"sasl_mechanism" mapstructure:"sasl_mechanism"` // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Username      string `yaml:"username" mapstructure:"username"`
	Password      string `yaml:"password" mapstructure:"password"`

	// Producer settings
	Compression  string `yaml:"compression" mapstructure:"compression"` // none, gzip, snappy, lz4, zstd
	BatchSize    int    `yaml:"batch_size" mapstructure:"batch_size"`
	BatchTimeout string `yaml:"batch_timeout" mapstructure:"batch_timeout"`
	RequiredAcks int    `yaml:"required_acks" mapstructure:"required_acks"`

	// Consumer protocol settings
	SessionTimeout    string `yaml:"session_timeout" mapstructure:"session_timeout"`
	HeartbeatInterval string `yaml:"heartbeat_interval" mapstructure:"heartbeat_interval"`
	RebalanceTimeout  string `yaml:"rebalance_timeout" mapstructure:"rebalance_timeout"`

	// Connection protocol settings
	DialTimeout string `yaml:"dial_timeout" mapstructure:"dial_timeout"`
	IdleTimeout string `yaml:"idle_timeout" mapstructure:"idle_timeout"`
	MetadataTTL string `yaml:"metadata_ttl" mapstructure:"metadata_ttl"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if len(c.Brokers) == 0 {
		c.Brokers = []string{"localhost:9092"}
	}
	if c.TLS == nil && !c.AllowInsecureDev {
		c.TLS = &security.TLSConfig{MinVersion: tls.VersionTLS12}
	}
	if c.Compression == "" {
		c.Compression = "snappy"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.BatchTimeout == "" {
		c.BatchTimeout = "1s"
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

// Validate checks that Kafka-specific fields are present and parseable.
func (c Config) Validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}
	for _, broker := range c.Brokers {
		if broker == "" {
			return fmt.Errorf("kafka broker address is required")
		}
	}
	if err := c.TLS.Validate(); err != nil {
		return fmt.Errorf("kafka tls: %w", err)
	}
	if !c.AllowInsecureDev && !c.TLS.IsEnabled() {
		return fmt.Errorf("kafka: TLS is required unless allow_insecure_dev is true")
	}
	for _, d := range []struct {
		name, val string
	}{
		{"batch_timeout", c.BatchTimeout},
		{"session_timeout", c.SessionTimeout},
		{"heartbeat_interval", c.HeartbeatInterval},
		{"rebalance_timeout", c.RebalanceTimeout},
		{"dial_timeout", c.DialTimeout},
		{"idle_timeout", c.IdleTimeout},
		{"metadata_ttl", c.MetadataTTL},
	} {
		if _, err := parsePositiveDuration(d.name, d.val); err != nil {
			return err
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
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be > 0")
	}
	return nil
}

// ValidateCommonProducer checks Kafka support for core producer semantics.
func ValidateCommonProducer(cfg messaging.Config) error {
	if cfg.DeliveryGuarantee == messaging.DeliveryExactlyOnce {
		return fmt.Errorf("kafka producer: exactly-once delivery requires transactions and is not supported")
	}
	if cfg.DLQ.Enabled {
		return fmt.Errorf("kafka producer: adapter-managed DLQ is not supported; use messaging middleware")
	}
	return nil
}

// ValidateCommonConsumer checks Kafka support for core consumer semantics.
func ValidateCommonConsumer(cfg messaging.Config) error {
	if cfg.DLQ.Enabled {
		return fmt.Errorf("kafka consumer: adapter-managed DLQ is not supported; use messaging middleware")
	}
	if cfg.MaxInFlight != 1 {
		return fmt.Errorf("kafka consumer: max_in_flight > 1 is not supported by the serial consumer")
	}
	switch cfg.DeliveryGuarantee {
	case messaging.DeliveryAtLeastOnce:
		if cfg.CommitStrategy != messaging.CommitAfterHandlerSuccess {
			return fmt.Errorf("kafka consumer: at-least-once delivery requires %s commits", messaging.CommitAfterHandlerSuccess)
		}
	case messaging.DeliveryAtMostOnce:
		if cfg.CommitStrategy != messaging.CommitAuto {
			return fmt.Errorf("kafka consumer: at-most-once delivery requires %s commits", messaging.CommitAuto)
		}
	case messaging.DeliveryExactlyOnce:
		return fmt.Errorf("kafka consumer: exactly-once delivery requires transactions and is not supported")
	}
	return nil
}

// ParseDuration parses a duration string, returning zero on empty input.
func ParseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func parsePositiveDuration(name, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, value, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be > 0", name)
	}
	return d, nil
}
