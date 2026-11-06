package lmstudio

import (
	"fmt"
	"strings"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Defaults and limits for [Config].
const (
	// DefaultTimeout bounds each request so a slow server cannot stall a listing.
	DefaultTimeout = 5 * time.Second
	// DefaultPollInterval spaces download status polls.
	DefaultPollInterval = time.Second
	// MinPollInterval is the shortest accepted poll interval.
	MinPollInterval = 100 * time.Millisecond
)

// Config configures a [Lister] or a [Downloader].
type Config struct {
	// BaseURL is the LM Studio server root, such as "http://localhost:1234".
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	// Timeout bounds each request. Defaults to [DefaultTimeout]. Must be zero when HTTP is set.
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`
	// PollInterval spaces [Downloader.Follow] status polls. Defaults to [DefaultPollInterval].
	PollInterval time.Duration `mapstructure:"poll_interval" json:"poll_interval"`
	// EndpointPolicy, when set, limits every request to its granted origins. Injected, never loaded from config.
	// Must be nil when HTTP is set.
	EndpointPolicy *security.EndpointPolicy `mapstructure:"-" json:"-"`
	// Auth, when set, sends the server's API token in a request header. Must be nil when HTTP is set.
	Auth *httpclient.AuthConfig `mapstructure:"-" json:"-"`
	// HTTP, when set, sends every request instead of an adapter built from Timeout, EndpointPolicy and Auth. It
	// must have no base URL and no retry policy. Injected, never loaded from config.
	HTTP *httpclient.Adapter `mapstructure:"-" json:"-"`
	// Clock drives download polls and status retries. Nil uses the monotonic system clock.
	Clock util.TimerClock `mapstructure:"-" json:"-"`
}

func (c Config) normalized() (Config, error) {
	base, err := security.NormalizeEndpointURL(c.BaseURL)
	if err != nil {
		return c, fmt.Errorf("lmstudio: base_url: %w", err)
	}
	c.BaseURL = strings.TrimRight(base, "/")
	if c.HTTP != nil {
		hc := c.HTTP.GetConfig()
		switch {
		case c.Timeout != 0 || c.EndpointPolicy != nil || c.Auth != nil:
			return c, fmt.Errorf("lmstudio: timeout, endpoint policy and auth belong to the injected http adapter")
		case hc.BaseURL != "" || (hc.ResiliencePolicy != nil && hc.ResiliencePolicy.Retry != nil):
			return c, fmt.Errorf("lmstudio: the injected http adapter must have no base url and no retry policy")
		}
	} else if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	switch {
	case c.PollInterval == 0:
		c.PollInterval = DefaultPollInterval
	case c.PollInterval < MinPollInterval:
		return c, fmt.Errorf("lmstudio: poll_interval must be at least %s", MinPollInterval)
	}
	if c.Clock == nil {
		c.Clock = util.MonotonicClock{}
	}
	return c, nil
}

// adapter returns the injected adapter or builds one from c.
func (c Config) adapter() (*httpclient.Adapter, error) {
	if c.HTTP != nil {
		return c.HTTP, nil
	}
	hc, err := httpclient.New(httpclient.Config{
		Name:                 "lmstudio",
		Timeout:              c.Timeout,
		Auth:                 c.Auth,
		EndpointPolicy:       c.EndpointPolicy,
		MaxResponseBodyBytes: maxBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("lmstudio: build http client: %w", err)
	}
	return hc, nil
}
