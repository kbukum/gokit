package nats

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/security"
)

const (
	adapterName = "nats"
	defaultURL  = "tls://localhost:4222"
)

// Config contains NATS-specific adapter settings.
type Config struct {
	URL              string              `yaml:"url" mapstructure:"url"`
	SubjectPrefix    string              `yaml:"subject_prefix" mapstructure:"subject_prefix"`
	QueueGroup       string              `yaml:"queue_group" mapstructure:"queue_group"`
	Queue            string              `yaml:"queue" mapstructure:"queue"`
	ConnectTimeout   string              `yaml:"connect_timeout" mapstructure:"connect_timeout"`
	PublishTimeout   string              `yaml:"publish_timeout" mapstructure:"publish_timeout"`
	DrainTimeout     string              `yaml:"drain_timeout" mapstructure:"drain_timeout"`
	MaxReconnects    int                 `yaml:"max_reconnects" mapstructure:"max_reconnects"`
	ReconnectWait    string              `yaml:"reconnect_wait" mapstructure:"reconnect_wait"`
	Token            string              `yaml:"token" mapstructure:"token"`
	Username         string              `yaml:"username" mapstructure:"username"`
	Password         string              `yaml:"password" mapstructure:"password"`
	TLS              *security.TLSConfig `yaml:"tls" mapstructure:"tls"`
	AllowInsecureDev bool                `yaml:"allow_insecure_dev" mapstructure:"allow_insecure_dev"`
}

// ApplyDefaults fills zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.URL == "" {
		c.URL = defaultURL
	}
	if c.QueueGroup == "" {
		c.QueueGroup = c.Queue
	}
	if c.ConnectTimeout == "" {
		c.ConnectTimeout = "10s"
	}
	if c.PublishTimeout == "" {
		c.PublishTimeout = "5s"
	}
	if c.DrainTimeout == "" {
		c.DrainTimeout = "30s"
	}
	if c.ReconnectWait == "" {
		c.ReconnectWait = "2s"
	}
}

// Validate checks NATS-specific settings.
func (c Config) Validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("nats: url is required")
	}
	for _, raw := range strings.Split(c.URL, ",") {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("nats: invalid url %q", raw)
		}
		if parsed.User != nil {
			return fmt.Errorf("nats: credentials must be configured via token or username/password fields, not URL userinfo")
		}
		if !c.AllowInsecureDev && parsed.Scheme != "tls" && parsed.Scheme != "wss" {
			return fmt.Errorf("nats: TLS URL scheme is required unless allow_insecure_dev is true")
		}
	}
	for _, value := range []struct{ name, val string }{
		{"connect_timeout", c.ConnectTimeout},
		{"publish_timeout", c.PublishTimeout},
		{"drain_timeout", c.DrainTimeout},
		{"reconnect_wait", c.ReconnectWait},
	} {
		if _, err := parsePositiveDuration("nats", value.name, value.val); err != nil {
			return err
		}
	}
	if c.MaxReconnects < -1 {
		return fmt.Errorf("nats: max_reconnects must be >= -1")
	}
	if c.Token != "" && (c.Username != "" || c.Password != "") {
		return fmt.Errorf("nats: token auth and username/password auth are mutually exclusive")
	}
	if c.Password != "" && c.Username == "" {
		return fmt.Errorf("nats: username is required when password is set")
	}
	if c.SubjectPrefix != "" {
		if err := messaging.ValidateTopic(strings.Trim(c.SubjectPrefix, ".")); err != nil {
			return fmt.Errorf("nats: invalid subject_prefix: %w", err)
		}
	}
	if c.QueueGroup != "" {
		if err := messaging.ValidateTopic(c.QueueGroup); err != nil {
			return fmt.Errorf("nats: invalid queue_group: %w", err)
		}
	}
	if err := c.TLS.Validate(); err != nil {
		return fmt.Errorf("nats tls: %w", err)
	}
	return nil
}

// RedactedURL returns URL with embedded credentials removed for logs/errors.
func (c Config) RedactedURL() string {
	parts := strings.Split(c.URL, ",")
	for i, raw := range parts {
		trimmed := strings.TrimSpace(raw)
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed.User != nil {
			parsed.User = url.User("[redacted]")
			parts[i] = parsed.String()
		} else {
			parts[i] = trimmed
		}
	}
	return strings.Join(parts, ",")
}

func (c Config) connectOptions() ([]natsgo.Option, error) {
	opts := []natsgo.Option{
		natsgo.Timeout(mustDuration(c.ConnectTimeout)),
		natsgo.DrainTimeout(mustDuration(c.DrainTimeout)),
		natsgo.ReconnectWait(mustDuration(c.ReconnectWait)),
		natsgo.MaxReconnects(c.MaxReconnects),
	}
	if c.Token != "" {
		opts = append(opts, natsgo.Token(c.Token))
	}
	if c.Username != "" || c.Password != "" {
		opts = append(opts, natsgo.UserInfo(c.Username, c.Password))
	}
	if c.TLS != nil && c.TLS.IsEnabled() {
		tlsCfg, err := c.TLS.Build()
		if err != nil {
			return nil, fmt.Errorf("nats tls: %w", err)
		}
		opts = append(opts, natsgo.Secure(tlsCfg))
	}
	return opts, nil
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

func subject(cfg Config, topic string) string {
	prefix := strings.Trim(cfg.SubjectPrefix, ".")
	if prefix == "" {
		return topic
	}
	return prefix + "." + topic
}
