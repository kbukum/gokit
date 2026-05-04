package rabbitmq

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/security"
)

const adapterName = "rabbitmq"

const defaultURL = "amqps://localhost:5671/"

var amqpCredentialPattern = regexp.MustCompile(`(amqps?)://[^/@\s]+@`)

// Config contains RabbitMQ-specific adapter settings.
type Config struct {
	URL               string              `yaml:"url" mapstructure:"url"`
	Username          string              `yaml:"username" mapstructure:"username"`
	Password          string              `yaml:"password" mapstructure:"password"`
	Exchange          string              `yaml:"exchange" mapstructure:"exchange"`
	ExchangeType      string              `yaml:"exchange_type" mapstructure:"exchange_type"`
	ExchangeDurable   bool                `yaml:"exchange_durable" mapstructure:"exchange_durable"`
	RoutingKeyPrefix  string              `yaml:"routing_key_prefix" mapstructure:"routing_key_prefix"`
	QueueName         string              `yaml:"queue_name" mapstructure:"queue_name"`
	QueuePrefix       string              `yaml:"queue_prefix" mapstructure:"queue_prefix"`
	QueueDurable      bool                `yaml:"queue_durable" mapstructure:"queue_durable"`
	AutoAck           bool                `yaml:"auto_ack" mapstructure:"auto_ack"`
	PrefetchCount     int                 `yaml:"prefetch_count" mapstructure:"prefetch_count"`
	ConnectionTimeout string              `yaml:"connection_timeout" mapstructure:"connection_timeout"`
	PublishTimeout    string              `yaml:"publish_timeout" mapstructure:"publish_timeout"`
	DeclareTimeout    string              `yaml:"declare_timeout" mapstructure:"declare_timeout"`
	Heartbeat         string              `yaml:"heartbeat" mapstructure:"heartbeat"`
	TLS               *security.TLSConfig `yaml:"tls" mapstructure:"tls"`
	AllowInsecureDev  bool                `yaml:"allow_insecure_dev" mapstructure:"allow_insecure_dev"`
}

// ApplyDefaults fills zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.URL == "" {
		c.URL = defaultURL
	}
	if c.ExchangeType == "" {
		c.ExchangeType = "direct"
	}
	if c.ConnectionTimeout == "" {
		c.ConnectionTimeout = "10s"
	}
	if c.PublishTimeout == "" {
		c.PublishTimeout = "5s"
	}
	if c.DeclareTimeout == "" {
		c.DeclareTimeout = "10s"
	}
	if c.Heartbeat == "" {
		c.Heartbeat = "10s"
	}
}

// Validate checks RabbitMQ-specific settings.
func (c Config) Validate() error {
	parsed, err := url.Parse(c.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("rabbitmq: invalid url")
	}
	if parsed.Scheme != "amqp" && parsed.Scheme != "amqps" {
		return fmt.Errorf("rabbitmq: url scheme must be amqp or amqps")
	}
	if parsed.User != nil {
		return fmt.Errorf("rabbitmq: credentials must be configured via username/password fields, not URL userinfo")
	}
	if c.Password != "" && c.Username == "" {
		return fmt.Errorf("rabbitmq: username is required when password is set")
	}
	if !c.AllowInsecureDev && parsed.Scheme != "amqps" {
		return fmt.Errorf("rabbitmq: amqps is required unless allow_insecure_dev is true")
	}
	switch c.ExchangeType {
	case "direct", "fanout", "topic", "headers":
	default:
		return fmt.Errorf("rabbitmq: unsupported exchange_type %q", c.ExchangeType)
	}
	for _, value := range []struct{ name, val string }{
		{"connection_timeout", c.ConnectionTimeout},
		{"publish_timeout", c.PublishTimeout},
		{"declare_timeout", c.DeclareTimeout},
		{"heartbeat", c.Heartbeat},
	} {
		if _, err := parsePositiveDuration("rabbitmq", value.name, value.val); err != nil {
			return err
		}
	}
	if c.PrefetchCount < 0 {
		return fmt.Errorf("rabbitmq: prefetch_count must be >= 0")
	}
	for _, value := range []struct{ name, val string }{
		{"routing_key_prefix", strings.Trim(c.RoutingKeyPrefix, ".")},
		{"queue_prefix", strings.Trim(c.QueuePrefix, ".")},
		{"queue_name", c.QueueName},
	} {
		if value.val != "" {
			if err := messaging.ValidateTopic(value.val); err != nil {
				return fmt.Errorf("rabbitmq: invalid %s: %w", value.name, err)
			}
		}
	}
	if err := c.TLS.Validate(); err != nil {
		return fmt.Errorf("rabbitmq tls: %w", err)
	}
	return nil
}

// RedactedURL returns URL with embedded credentials removed for logs/errors.
func (c Config) RedactedURL() string {
	parsed, err := c.connectionURL()
	if err != nil || parsed.User == nil {
		return c.URL
	}
	parsed.User = url.User("[redacted]")
	return parsed.String()
}

func (c Config) connectionURL() (*url.URL, error) {
	parsed, err := url.Parse(c.URL)
	if err != nil {
		return nil, err
	}
	if c.Username != "" {
		parsed.User = url.UserPassword(c.Username, c.Password)
	}
	return parsed, nil
}

func parsePositiveDuration(prefix, name, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid %s %q: %w", prefix, name, value, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: %s must be > 0", prefix, name)
	}
	return d, nil
}

func mustDuration(value string) time.Duration {
	d, _ := time.ParseDuration(value)
	return d
}

type redactedError struct {
	op  string
	err error
}

func redactError(op string, err error) error {
	if err == nil {
		return nil
	}
	return redactedError{op: op, err: err}
}

func (e redactedError) Error() string {
	return e.op + ": " + amqpCredentialPattern.ReplaceAllString(e.err.Error(), "$1://[redacted]@")
}

func (e redactedError) Unwrap() error {
	return e.err
}
