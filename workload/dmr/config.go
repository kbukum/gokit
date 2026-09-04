package dmr

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kbukum/gokit/resilience"
)

// DefaultBaseURL is the Docker Model Runner host TCP endpoint on Docker Desktop
// and Docker Engine. From inside a container, use
// "http://model-runner.docker.internal" instead.
const DefaultBaseURL = "http://localhost:12434"

// DefaultTimeout bounds a single non-streaming DMR request so a call made with a
// deadline-less context (e.g. context.Background()) cannot hang indefinitely.
// Model pulls stream progress and are governed by the request context instead.
const DefaultTimeout = 30 * time.Second

// DefaultPullTimeout bounds a model pull when the caller's context carries no
// deadline. A pull streams progress and can be long-running (multi-gigabyte
// downloads), so this is generous; a caller that supplies its own deadline takes
// precedence, and a byte cap alone cannot bound an idle stream that stalls
// without producing bytes.
const DefaultPullTimeout = 30 * time.Minute

// Config holds Docker Model Runner backend configuration.
type Config struct {
	// BaseURL is the DMR API root (no trailing "/engines/v1"). Defaults to
	// [DefaultBaseURL].
	BaseURL string `mapstructure:"base_url" json:"base_url"`

	// Timeout bounds each non-streaming request. Defaults to [DefaultTimeout].
	Timeout time.Duration `mapstructure:"timeout" json:"timeout"`

	// PullTimeout bounds a model pull when the incoming context has no deadline,
	// so a stalled create stream cannot hang a deadline-less context forever.
	// Defaults to [DefaultPullTimeout]; an earlier caller deadline is preserved.
	PullTimeout time.Duration `mapstructure:"pull_timeout" json:"pull_timeout"`

	// ResiliencePolicy optionally wraps non-streaming requests with retry,
	// circuit-breaking, and rate limiting. Timeout/retry/circuit-break are owned
	// by the canonical resilience package; leave nil for a plain
	// timeout-bounded client.
	ResiliencePolicy *resilience.Policy `mapstructure:"-" json:"-"`
}

// ApplyDefaults fills in zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.PullTimeout <= 0 {
		c.PullTimeout = DefaultPullTimeout
	}
}

// Validate checks the Docker Model Runner configuration.
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("dmr: base_url is required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("dmr: invalid base_url %q: %w", c.BaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("dmr: base_url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("dmr: base_url must include a host, got %q", c.BaseURL)
	}
	// BaseURL is reused verbatim in the public inference endpoint (see
	// ModelHandle.Endpoint) and in logs. DMR is unauthenticated, so embedded
	// userinfo would only leak credentials through the endpoint and logs.
	if u.User != nil {
		return fmt.Errorf("dmr: base_url must not carry userinfo credentials")
	}
	// Request paths are appended to base_url, so a query or fragment on the root
	// would corrupt every derived request target.
	if u.RawQuery != "" || u.ForceQuery {
		return fmt.Errorf("dmr: base_url must not carry a query string, got %q", c.BaseURL)
	}
	if u.Fragment != "" {
		return fmt.Errorf("dmr: base_url must not carry a fragment, got %q", c.BaseURL)
	}
	return nil
}

// normalizedBaseURL returns BaseURL without any trailing slashes, so request
// paths appended to it (for example "/models") never produce a doubled "//".
func (c *Config) normalizedBaseURL() string {
	return strings.TrimRight(c.BaseURL, "/")
}
