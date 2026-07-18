package discovery

import (
	"google.golang.org/grpc/resolver"

	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
)

const defaultScheme = "consul"

// Option configures the resolver builder.
type Option func(*options)

type options struct {
	scheme string
}

// WithScheme overrides the resolver scheme (default: "consul").
func WithScheme(scheme string) Option {
	return func(o *options) { o.scheme = scheme }
}

// ResolverBuilder implements resolver.Builder using a discovery.Discovery backend.
type ResolverBuilder struct {
	discovery discovery.Discovery
	log       *logging.Logger
	scheme    string
}

// NewResolverBuilder creates a resolver builder backed by the given Discovery provider.
func NewResolverBuilder(disc discovery.Discovery, log *logging.Logger, opts ...Option) *ResolverBuilder {
	o := &options{scheme: defaultScheme}
	for _, opt := range opts {
		opt(o)
	}
	if log == nil {
		log = logging.Default()
	}
	return &ResolverBuilder{
		discovery: disc,
		log:       log,
		scheme:    o.scheme,
	}
}

// Build creates a new resolver for the given target. target.Endpoint() returns the service name (e.g., "ssm-ingestion" from "consul:///ssm-ingestion").
func (b *ResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()

	b.log.Debug("Building discovery resolver", map[string]any{
		"service": serviceName,
		"scheme":  b.scheme,
	})

	r := newDiscoveryResolver(serviceName, b.discovery, cc, b.log)
	r.start()
	return r, nil
}

// Scheme returns the resolver scheme (e.g., "consul").
func (b *ResolverBuilder) Scheme() string {
	return b.scheme
}

// Compile-time check.
var _ resolver.Builder = (*ResolverBuilder)(nil)
