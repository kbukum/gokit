package oci

import (
	"strings"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Defaults for [Config].
const (
	DefaultRegistry         = "https://registry-1.docker.io"
	DefaultMaxManifestBytes = 4 << 20
	DefaultMaxConfigBytes   = 4 << 20
	DefaultPageSize         = 100
	DefaultMaxTags          = 1000
	DefaultMaxTokens        = 64
	maxTokenBytes           = 64 << 10
	maxLayers               = 4096
)

// Config configures a [Client].
type Config struct {
	// HTTP sends every request. It must have no base URL, no retry policy and no auth: the client owns retries and
	// registry tokens. It must have an endpoint policy granting the registry, its token realm and its blob CDN, with
	// EndpointRedirects for blob redirects; nothing else is reachable. For Docker Hub that is registry-1.docker.io,
	// auth.docker.io and production.cloudfront.docker.com (seen 2026-11); a refused redirect is a typed Forbidden.
	HTTP *httpclient.Adapter
	// Clock dates results, expires tokens and drives retry backoff. Nil uses the monotonic system clock.
	Clock util.TimerClock
	// Retry bounds retries of idempotent reads. The zero value uses three attempts within 30 seconds.
	Retry resilience.RetryConfig
	// Registry is the registry origin. Defaults to [DefaultRegistry].
	Registry string
	// MaxManifestBytes bounds a manifest body. Defaults to [DefaultMaxManifestBytes].
	MaxManifestBytes int64
	// MaxConfigBytes bounds a config blob. Defaults to [DefaultMaxConfigBytes].
	MaxConfigBytes int64
	// PageSize is the tags requested per page. Defaults to [DefaultPageSize].
	PageSize int
	// MaxTags bounds the tags [Client.Tags] returns. Defaults to [DefaultMaxTags].
	MaxTags int
	// MaxTokens bounds the cached registry tokens. Defaults to [DefaultMaxTokens].
	MaxTokens int
}

// Client reads model artifacts from one registry. It is safe for concurrent use.
type Client struct {
	fetch    *fetch.Client
	registry string
	cfg      Config
	tokens   *tokenCache
}

// New validates cfg and builds a client.
func New(cfg Config) (*Client, error) {
	if cfg.HTTP == nil {
		return nil, apperr.MissingField("http")
	}
	if cfg.HTTP.GetConfig().Auth != nil {
		return nil, apperr.InvalidInput("http", "The adapter must not carry auth; the client owns registry tokens")
	}
	if cfg.Registry == "" {
		cfg.Registry = DefaultRegistry
	}
	registry, err := origin(cfg.Registry)
	if err != nil {
		return nil, err
	}
	defaults(&cfg.MaxManifestBytes, DefaultMaxManifestBytes)
	defaults(&cfg.MaxConfigBytes, DefaultMaxConfigBytes)
	defaults(&cfg.PageSize, DefaultPageSize)
	defaults(&cfg.MaxTags, DefaultMaxTags)
	defaults(&cfg.MaxTokens, DefaultMaxTokens)
	if cfg.MaxManifestBytes < 0 || cfg.MaxConfigBytes < 0 || cfg.PageSize < 0 || cfg.MaxTags < 0 || cfg.MaxTokens < 0 {
		return nil, apperr.InvalidInput("config", "Bounds must not be negative")
	}
	f, err := fetch.New("model registry", fetch.Config{HTTP: cfg.HTTP, Clock: cfg.Clock, Retry: cfg.Retry})
	if err != nil {
		return nil, err
	}
	return &Client{fetch: f, registry: registry, cfg: cfg, tokens: newTokenCache(cfg.MaxTokens)}, nil
}

func defaults[T int | int64](v *T, def T) {
	if *v == 0 {
		*v = def
	}
}

// origin normalizes an HTTP(S) origin without a path.
func origin(raw string) (string, error) {
	normalized, err := security.NormalizeEndpointURL(raw)
	if err != nil {
		return "", err
	}
	normalized = strings.TrimRight(normalized, "/")
	if strings.Count(normalized, "/") != 2 {
		return "", apperr.InvalidInput("registry", "An origin without a path is required")
	}
	return normalized, nil
}
