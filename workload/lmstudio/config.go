package lmstudio

import (
	"fmt"
	"strings"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
)

// DefaultTimeout bounds each request so a slow server cannot stall a listing.
const DefaultTimeout = 5 * time.Second

// Config configures a [Lister].
type Config struct {
	// BaseURL is the LM Studio server root, such as "http://localhost:1234".
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	// Timeout bounds each request. Defaults to [DefaultTimeout].
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`
	// EndpointPolicy, when set, limits every request to its granted origins. Injected, never loaded from config.
	EndpointPolicy *security.EndpointPolicy `mapstructure:"-" json:"-"`
	// Auth, when set, sends the server's API token in a request header.
	Auth *httpclient.AuthConfig `mapstructure:"-" json:"-"`
}

func (c Config) normalized() (Config, error) {
	base, err := security.NormalizeEndpointURL(c.BaseURL)
	if err != nil {
		return c, fmt.Errorf("lmstudio: base_url: %w", err)
	}
	c.BaseURL = strings.TrimRight(base, "/")
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c, nil
}
