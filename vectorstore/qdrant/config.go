package qdrant

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kbukum/gokit/vectorstore"
)

// ProviderName is the canonical provider name for Qdrant.
const ProviderName = "qdrant"

// DefaultURL is the default Qdrant REST endpoint.
const DefaultURL = "http://localhost:6333"

// Config holds Qdrant-specific configuration.
type Config struct {
	// URL is the Qdrant REST endpoint. It must not include credentials, query, or fragment.
	URL string `mapstructure:"url" json:"url"`
	// APIKey is the optional Qdrant Cloud API key sent in the api-key header.
	APIKey string `mapstructure:"api_key" json:"-"`
	// Metric overrides the core metric when creating collections if set.
	Metric string `mapstructure:"metric" json:"metric"`
	// Timeout bounds each remote HTTP operation.
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`
}

// ApplyDefaults fills zero-valued fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.URL == "" {
		c.URL = DefaultURL
	}
	if c.Metric == "" {
		c.Metric = vectorstore.MetricCosine
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
}

// Validate checks that the Qdrant configuration is valid.
func (c *Config) Validate() error {
	var errs []error
	if err := validateEndpoint(c.URL); err != nil {
		errs = append(errs, err)
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("qdrant: timeout must be positive"))
	}
	switch c.Metric {
	case vectorstore.MetricCosine, vectorstore.MetricDot, vectorstore.MetricL2:
	default:
		errs = append(errs, &vectorstore.MetricError{Metric: c.Metric})
	}
	if len(errs) > 0 {
		return fmt.Errorf("qdrant: invalid config: %w", errors.Join(errs...))
	}
	return nil
}

func validateEndpoint(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("qdrant: url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("qdrant: invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("qdrant: url scheme must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("qdrant: url host is required")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("qdrant: url must not contain credentials, query parameters, or fragments")
	}
	return nil
}
