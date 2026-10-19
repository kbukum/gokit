package security

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// EndpointRule grants one exact origin; private destinations require explicit subnet permissions.
type EndpointRule struct {
	Origin        string
	AllowHTTP     bool
	PrivateRanges []netip.Prefix
}

// EndpointResolver resolves every candidate address using the operation context.
type EndpointResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// EndpointDialer opens only the numeric address approved by the policy.
type EndpointDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// EndpointPolicy owns immutable origin grants and checks DNS again at every connection. Install DialContext on a proxy-free transport that retains hostname certificate verification/SNI and refuses redirects.
type EndpointPolicy struct {
	rules    map[string]EndpointRule
	resolver EndpointResolver
	dialer   EndpointDialer
	budget   time.Duration
}

// Endpoint connection budgets.
const (
	// DefaultEndpointDialBudget bounds DNS resolution plus connection setup when no budget is configured.
	DefaultEndpointDialBudget = 10 * time.Second
	// MaxEndpointDialBudget is the largest accepted dial budget.
	MaxEndpointDialBudget = 2 * time.Minute
)

// EndpointOption configures an [EndpointPolicy].
type EndpointOption func(*EndpointPolicy) error

// WithEndpointDialBudget bounds DNS resolution plus connection setup for one dial, across every candidate address. It must be positive and at most [MaxEndpointDialBudget]; the default is [DefaultEndpointDialBudget].
func WithEndpointDialBudget(budget time.Duration) EndpointOption {
	return func(p *EndpointPolicy) error {
		if budget <= 0 || budget > MaxEndpointDialBudget {
			return apperrors.InvalidInput("endpoint policy", "The dial budget must be positive and at most two minutes")
		}
		p.budget = budget
		return nil
	}
}

// NewEndpointPolicy copies at most 128 origin grants, with at most 32 private subnet grants per origin.
func NewEndpointPolicy(rules []EndpointRule, resolver EndpointResolver, dialer EndpointDialer, opts ...EndpointOption) (*EndpointPolicy, error) {
	if len(rules) == 0 || len(rules) > 128 || util.IsNil(resolver) || util.IsNil(dialer) {
		return nil, apperrors.InvalidInput("endpoint policy", "Bounded explicit origins, a resolver and a dialer are required")
	}
	p := &EndpointPolicy{rules: make(map[string]EndpointRule, len(rules)), resolver: resolver, dialer: dialer, budget: DefaultEndpointDialBudget}
	for _, opt := range opts {
		if opt == nil {
			return nil, apperrors.InvalidInput("endpoint policy", "Options must not be nil")
		}
		if err := opt(p); err != nil {
			return nil, err
		}
	}
	for _, rule := range rules {
		u, err := endpointURL(rule.Origin)
		if err != nil {
			return nil, err
		}
		if (u.Path != "" && u.Path != "/") || (u.Scheme == "http" && !rule.AllowHTTP) || len(rule.PrivateRanges) > 32 {
			return nil, apperrors.InvalidInput("endpoint policy", "Rules require an origin, explicit HTTP permission and bounded private ranges")
		}
		for _, subnet := range rule.PrivateRanges {
			if !privateSubnet(subnet) {
				return nil, apperrors.InvalidInput("endpoint policy", "Private permissions must be subnets of private, loopback or link-local ranges")
			}
		}
		origin := u.Scheme + "://" + u.Host
		_, httpsExists := p.rules["https://"+u.Host]
		_, httpExists := p.rules["http://"+u.Host]
		if httpsExists || httpExists {
			return nil, apperrors.InvalidInput("endpoint policy", "Each authority requires one unambiguous scheme grant")
		}
		rule.Origin, rule.PrivateRanges = origin, slices.Clone(rule.PrivateRanges)
		p.rules[origin] = rule
	}
	return p, nil
}

// Validate returns the normalized endpoint after validating all current DNS answers within the dial budget. Connection-time validation remains mandatory; validation here cannot pin a later DNS lookup.
func (p *EndpointPolicy) Validate(ctx context.Context, raw string) (string, error) {
	if util.IsNil(ctx) {
		return "", apperrors.InvalidInput("endpoint", "A context is required")
	}
	normalized, err := p.Normalize(raw)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return "", apperrors.Internal(err)
	}
	rule := p.rules[u.Scheme+"://"+u.Host]
	ctx, cancel := context.WithTimeout(ctx, p.budget)
	defer cancel()
	if _, err := p.addresses(ctx, u.Hostname(), rule); err != nil {
		return "", err
	}
	return normalized, nil
}

// Normalize validates syntax and an explicit origin grant without DNS or I/O. It does not authorize a connection; Validate and DialContext enforce resolved address permissions.
func (p *EndpointPolicy) Normalize(raw string) (string, error) {
	u, err := endpointURL(raw)
	if err != nil {
		return "", err
	}
	_, ok := p.rules[u.Scheme+"://"+u.Host]
	if !ok {
		return "", apperrors.Forbidden("Endpoint origin is not permitted")
	}
	return u.String(), nil
}

// DialContext resolves at most sixteen candidates and validates all of them before dialing their numeric addresses in resolver order until one connects. DNS and every connection attempt share the dial budget; no unchecked DNS lookup follows validation.
func (p *EndpointPolicy) DialContext(ctx context.Context, network, authority string) (net.Conn, error) {
	if util.IsNil(ctx) {
		return nil, apperrors.InvalidInput("endpoint", "A context is required")
	}
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, apperrors.InvalidInput("endpoint", "Only TCP connections are permitted")
	}
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, apperrors.InvalidInput("endpoint", "A host and explicit port are required").WithCause(err)
	}
	host = strings.ToLower(host)
	authority = net.JoinHostPort(host, port)
	var rule EndpointRule
	var permitted bool
	for _, scheme := range []string{"https", "http"} {
		if grant, ok := p.rules[scheme+"://"+authority]; ok {
			rule, permitted = grant, true
			break
		}
	}
	if !permitted {
		return nil, apperrors.Forbidden("Connection authority is not permitted")
	}
	ctx, cancel := context.WithTimeout(ctx, p.budget)
	defer cancel()
	addresses, err := p.addresses(ctx, host, rule)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, address := range addresses {
		if (network == "tcp4" && !address.Is4()) || (network == "tcp6" && address.Is4()) {
			continue
		}
		conn, dialErr := p.dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if dialErr == nil && util.IsNil(conn) {
			dialErr = apperrors.Internal(errors.New("endpoint dialer returned no connection"))
		}
		if dialErr == nil {
			return conn, nil
		}
		if !util.IsNil(conn) {
			dialErr = errors.Join(dialErr, conn.Close())
		}
		failures = append(failures, dialErr)
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 {
		return nil, apperrors.Forbidden("No permitted address matches the requested network")
	}
	return nil, errors.Join(failures...)
}

func (p *EndpointPolicy) addresses(ctx context.Context, host string, rule EndpointRule) ([]netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var addresses []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{literal}
	} else {
		var err error
		addresses, err = p.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, apperrors.New(apperrors.ErrCodeServiceUnavailable, "Endpoint DNS resolution failed").WithCause(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(addresses) == 0 || len(addresses) > 16 {
		return nil, apperrors.Forbidden("Endpoint resolution must return one to sixteen addresses")
	}
	addresses = slices.Clone(addresses)
	for i, candidate := range addresses {
		address := candidate.Unmap()
		class := classifyAddress(address)
		if class == addressRefused {
			return nil, apperrors.Forbidden("Endpoint resolved an invalid destination")
		}
		if class == addressNonPublic {
			allowed := false
			for _, subnet := range rule.PrivateRanges {
				allowed = allowed || subnet.Contains(address)
			}
			if !allowed {
				return nil, apperrors.Forbidden("Endpoint resolved an unapproved private destination")
			}
		}
		addresses[i] = address
	}
	return addresses, nil
}

// NormalizeEndpointURL canonicalizes bounded HTTP(S) syntax without granting network permission or performing I/O.
func NormalizeEndpointURL(raw string) (string, error) {
	u, err := endpointURL(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func endpointURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u == nil || (u.Scheme != "https" && u.Scheme != "http") ||
		u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(raw, "?#") ||
		u.RawPath != "" || strings.HasSuffix(u.Host, ":") || strings.HasSuffix(u.Hostname(), ".") {
		return nil, apperrors.InvalidInput("endpoint", "A bounded unambiguous HTTP(S) URL without userinfo, query or fragment is required")
	}
	host := strings.ToLower(u.Hostname())
	if ip, ipErr := netip.ParseAddr(host); ipErr == nil {
		if ip.Zone() != "" {
			return nil, apperrors.InvalidInput("endpoint", "Address zones are not permitted")
		}
		host = ip.Unmap().String()
	} else {
		if len(host) > 253 {
			return nil, apperrors.InvalidInput("endpoint", "Hostname is too long")
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, apperrors.InvalidInput("endpoint", "Hostname must use canonical DNS labels")
			}
			for _, c := range label {
				if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
					return nil, apperrors.InvalidInput("endpoint", "Hostname must use ASCII DNS labels")
				}
			}
		}
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return nil, apperrors.InvalidInput("endpoint", "Port must be between 1 and 65535")
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(number))
	return u, nil
}

func privateSubnet(subnet netip.Prefix) bool {
	if !subnet.IsValid() || subnet != subnet.Masked() {
		return false
	}
	for _, block := range nonPublicBlocks {
		if subnet.Bits() >= block.Bits() && block.Contains(subnet.Addr()) {
			return true
		}
	}
	return false
}
