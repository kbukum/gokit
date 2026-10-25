package ollama

import (
	"fmt"
	"strings"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
)

// DefaultTimeout bounds each request so a slow server cannot stall a listing.
const DefaultTimeout = 5 * time.Second

// DefaultMaxDescribed bounds how many models get a POST /api/show call per listing. Models past the bound are
// listed with what /api/tags states.
const DefaultMaxDescribed = 64

// DefaultConcurrency is how many POST /api/show calls a listing makes at once.
const DefaultConcurrency = 4

// MaxConcurrency bounds Concurrency so one listing cannot flood a local server.
const MaxConcurrency = 16

// Config configures a [Lister].
type Config struct {
	// BaseURL is the Ollama API root, such as "http://localhost:11434".
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	// Timeout bounds each request. Defaults to [DefaultTimeout].
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`
	// MaxDescribed bounds the per-model show calls. Zero means [DefaultMaxDescribed].
	MaxDescribed int `mapstructure:"max_described" json:"max_described"`
	// Concurrency bounds the show calls in flight at once, at most [MaxConcurrency]. Zero means [DefaultConcurrency].
	Concurrency int `mapstructure:"concurrency" json:"concurrency"`
	// EndpointPolicy, when set, limits every request to its granted origins. Injected, never loaded from config.
	EndpointPolicy *security.EndpointPolicy `mapstructure:"-" json:"-"`
	// Auth, when set, sends a credential in a request header, for a server behind an authenticating proxy.
	Auth *httpclient.AuthConfig `mapstructure:"-" json:"-"`
}

func (c Config) normalized() (Config, error) {
	base, err := security.NormalizeEndpointURL(c.BaseURL)
	if err != nil {
		return c, fmt.Errorf("ollama: base_url: %w", err)
	}
	c.BaseURL = strings.TrimRight(base, "/")
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	switch {
	case c.MaxDescribed < 0:
		return c, fmt.Errorf("ollama: max_described must not be negative")
	case c.MaxDescribed == 0:
		c.MaxDescribed = DefaultMaxDescribed
	}
	switch {
	case c.Concurrency < 0 || c.Concurrency > MaxConcurrency:
		return c, fmt.Errorf("ollama: concurrency must be between 0 and %d", MaxConcurrency)
	case c.Concurrency == 0:
		c.Concurrency = DefaultConcurrency
	}
	return c, nil
}
