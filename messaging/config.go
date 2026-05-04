package messaging

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultBackend        = "memory"
	DefaultMaxInFlight    = 1
	DefaultDLQSuffix      = ".dlq"
	DefaultRequestTimeout = "30s"
	DefaultRetryAttempts  = 3
	DefaultRetryBackoff   = "100ms"
)

// Config contains transport-agnostic messaging semantics.
type Config struct {
	Name              string            `yaml:"name" mapstructure:"name"`
	Backend           string            `yaml:"backend" mapstructure:"backend"`
	Enabled           *bool             `yaml:"enabled" mapstructure:"enabled"`
	DeliveryGuarantee DeliveryGuarantee `yaml:"delivery_guarantee" mapstructure:"delivery_guarantee"`
	CommitStrategy    CommitStrategy    `yaml:"commit_strategy" mapstructure:"commit_strategy"`
	DLQ               DLQPolicy         `yaml:"dlq" mapstructure:"dlq"`
	MaxInFlight       int               `yaml:"max_in_flight" mapstructure:"max_in_flight"`
	ConsumerGroup     string            `yaml:"consumer_group" mapstructure:"consumer_group"`
	Topics            []string          `yaml:"topics" mapstructure:"topics"`
	Subscriptions     []string          `yaml:"subscriptions" mapstructure:"subscriptions"`
	RequestTimeout    string            `yaml:"request_timeout" mapstructure:"request_timeout"`
	RetryAttempts     int               `yaml:"retry_attempts" mapstructure:"retry_attempts"`
	RetryBackoff      string            `yaml:"retry_backoff" mapstructure:"retry_backoff"`
}

// ApplyDefaults fills zero-valued config fields with deterministic safe defaults.
func (c *Config) ApplyDefaults() {
	if c.Backend == "" {
		c.Backend = DefaultBackend
	}
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.DeliveryGuarantee == "" {
		c.DeliveryGuarantee = DeliveryAtLeastOnce
	}
	if c.CommitStrategy == "" {
		c.CommitStrategy = CommitAfterHandlerSuccess
	}
	if c.MaxInFlight <= 0 {
		c.MaxInFlight = DefaultMaxInFlight
	}
	if c.RequestTimeout == "" {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.RetryAttempts == 0 {
		c.RetryAttempts = DefaultRetryAttempts
	}
	if c.RetryBackoff == "" {
		c.RetryBackoff = DefaultRetryBackoff
	}
	c.DLQ.ApplyDefaults()
}

// IsEnabled reports whether this messaging config is active.
func (c Config) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// Validate checks transport-agnostic messaging settings.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Backend) == "" {
		return fmt.Errorf("messaging: backend is required")
	}
	if !validName(c.Backend) {
		return fmt.Errorf("messaging: backend %q must contain only letters, digits, '.', '_' or '-'", c.Backend)
	}
	if c.Name != "" && !validName(c.Name) {
		return fmt.Errorf("messaging: name %q must contain only letters, digits, '.', '_' or '-'", c.Name)
	}
	if err := c.DeliveryGuarantee.Validate(); err != nil {
		return err
	}
	if err := c.CommitStrategy.Validate(); err != nil {
		return err
	}
	if c.MaxInFlight <= 0 {
		return fmt.Errorf("messaging: max_in_flight must be > 0")
	}
	if c.RetryAttempts < 0 {
		return fmt.Errorf("messaging: retry_attempts must be >= 0")
	}
	if c.ConsumerGroup != "" {
		if err := ValidateTopic(c.ConsumerGroup); err != nil {
			return fmt.Errorf("messaging: invalid consumer_group: %w", err)
		}
	}
	for _, topic := range c.Topics {
		if err := ValidateTopic(topic); err != nil {
			return fmt.Errorf("messaging: invalid topic: %w", err)
		}
	}
	for _, subscription := range c.Subscriptions {
		if err := ValidateTopic(subscription); err != nil {
			return fmt.Errorf("messaging: invalid subscription: %w", err)
		}
	}
	if err := validateDuration("request_timeout", c.RequestTimeout); err != nil {
		return err
	}
	if err := validateDuration("retry_backoff", c.RetryBackoff); err != nil {
		return err
	}
	return c.DLQ.Validate()
}

// DeliveryGuarantee describes the requested broker delivery semantics.
type DeliveryGuarantee string

const (
	DeliveryAtMostOnce  DeliveryGuarantee = "at_most_once"
	DeliveryAtLeastOnce DeliveryGuarantee = "at_least_once"
	DeliveryExactlyOnce DeliveryGuarantee = "exactly_once"
)

// Validate checks that the delivery guarantee is known.
func (g DeliveryGuarantee) Validate() error {
	switch g {
	case DeliveryAtMostOnce, DeliveryAtLeastOnce, DeliveryExactlyOnce:
		return nil
	default:
		return fmt.Errorf("messaging: unsupported delivery_guarantee %q", g)
	}
}

// CommitStrategy describes when consumed offsets/acks are committed.
type CommitStrategy string

const (
	CommitAuto                CommitStrategy = "auto"
	CommitAfterHandlerSuccess CommitStrategy = "post_handler_success"
	CommitManual              CommitStrategy = "manual"
)

// Validate checks that the commit strategy is known.
func (s CommitStrategy) Validate() error {
	switch s {
	case CommitAuto, CommitAfterHandlerSuccess, CommitManual:
		return nil
	default:
		return fmt.Errorf("messaging: unsupported commit_strategy %q", s)
	}
}

// DLQPolicy configures dead-letter routing.
type DLQPolicy struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Suffix  string `yaml:"suffix" mapstructure:"suffix"`
}

// ApplyDefaults fills zero-valued DLQ policy fields. Enabled defaults to false.
func (p *DLQPolicy) ApplyDefaults() {
	if p.Suffix == "" {
		p.Suffix = DefaultDLQSuffix
	}
}

// Validate checks dead-letter routing settings.
func (p DLQPolicy) Validate() error {
	if strings.TrimSpace(p.Suffix) == "" {
		return fmt.Errorf("messaging: dlq suffix is required")
	}
	if strings.ContainsFunc(p.Suffix, unicode.IsSpace) {
		return fmt.Errorf("messaging: dlq suffix must not contain whitespace")
	}
	return nil
}

// ValidateTopic checks broker-neutral topic/subject/group names.
func ValidateTopic(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("messaging: topic is required")
	}
	if len(value) > 249 {
		return fmt.Errorf("messaging: topic %q must be at most 249 bytes", value)
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r) || r == '/' || r == '\\'
	}) {
		return fmt.Errorf("messaging: topic %q must not contain whitespace, control characters, or path separators", value)
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == ':' {
			continue
		}
		return fmt.Errorf("messaging: topic %q contains unsupported character %q", value, r)
	}
	return nil
}

func validateDuration(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("messaging: %s is required", name)
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("messaging: invalid %s %q: %w", name, value, err)
	}
	if d <= 0 {
		return fmt.Errorf("messaging: %s must be > 0", name)
	}
	return nil
}

func validName(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
