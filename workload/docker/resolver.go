package docker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logger"
)

// HostResolver resolves the Docker daemon host for a given context.
// Implementations can look up per-tenant or per-workspace hosts.
type HostResolver interface {
	ResolveHost(ctx context.Context) (string, error)
}

// HostResolverFunc adapts a plain function to HostResolver.
type HostResolverFunc func(ctx context.Context) (string, error)

// ResolveHost calls the underlying function.
func (f HostResolverFunc) ResolveHost(ctx context.Context) (string, error) { return f(ctx) }

// StaticResolver always returns the same host.
type StaticResolver struct{ Host string }

// ResolveHost returns the static host.
func (r StaticResolver) ResolveHost(context.Context) (string, error) { return r.Host, nil }

// cachedManager holds a Manager and its expiry.
type cachedManager struct {
	manager   *Manager
	host      string
	expiresAt time.Time
}

// ManagerPool manages multiple Docker Managers keyed by resolved host.
// It caches managers and creates new ones on-demand via the HostResolver.
type ManagerPool struct {
	mu            sync.RWMutex
	resolver      HostResolver
	defaultMgr    *Manager
	cache         map[string]*cachedManager
	cacheTTL      time.Duration
	cfg           *Config
	defaultLabels map[string]string
	log           *logger.Logger
}

// ManagerPoolOption configures a ManagerPool.
type ManagerPoolOption func(*ManagerPool)

// WithCacheTTL sets how long a cached Manager remains valid.
func WithCacheTTL(ttl time.Duration) ManagerPoolOption {
	return func(p *ManagerPool) { p.cacheTTL = ttl }
}

// NewManagerPool creates a pool that resolves Docker hosts dynamically.
// When resolver is nil, it behaves like a single-host manager.
func NewManagerPool(cfg *Config, resolver HostResolver, defaultLabels map[string]string, log *logger.Logger, opts ...ManagerPoolOption) (*ManagerPool, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	defaultMgr, err := NewManager(cfg, defaultLabels, log)
	if err != nil {
		return nil, fmt.Errorf("docker: create default manager: %w", err)
	}

	pool := &ManagerPool{
		resolver:      resolver,
		defaultMgr:    defaultMgr,
		cache:         make(map[string]*cachedManager),
		cacheTTL:      5 * time.Minute,
		cfg:           cfg,
		defaultLabels: defaultLabels,
		log:           log,
	}
	for _, opt := range opts {
		opt(pool)
	}
	return pool, nil
}

// For returns the Manager for the current context. If a HostResolver is
// configured and resolves to a different host, a cached manager for that
// host is returned (or created).
func (p *ManagerPool) For(ctx context.Context) (*Manager, error) {
	if p.resolver == nil {
		return p.defaultMgr, nil
	}

	host, err := p.resolver.ResolveHost(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker: resolve host: %w", err)
	}
	if host == "" || host == p.cfg.Host {
		return p.defaultMgr, nil
	}

	return p.managerForHost(host)
}

// Default returns the default (non-resolved) Manager.
func (p *ManagerPool) Default() *Manager {
	return p.defaultMgr
}

func (p *ManagerPool) managerForHost(host string) (*Manager, error) {
	p.mu.RLock()
	if cm, ok := p.cache[host]; ok && time.Now().Before(cm.expiresAt) {
		p.mu.RUnlock()
		return cm.manager, nil
	}
	p.mu.RUnlock()

	cfg := *p.cfg
	cfg.Host = host
	mgr, err := NewManager(&cfg, p.defaultLabels, p.log)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.cache[host] = &cachedManager{
		manager:   mgr,
		host:      host,
		expiresAt: time.Now().Add(p.cacheTTL),
	}
	p.mu.Unlock()
	return mgr, nil
}

// Invalidate removes the cached manager for the given host.
func (p *ManagerPool) Invalidate(host string) {
	p.mu.Lock()
	delete(p.cache, host)
	p.mu.Unlock()
}

// Close releases all cached managers.
func (p *ManagerPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, cm := range p.cache {
		_ = cm.manager.client.Close()
	}
	p.cache = make(map[string]*cachedManager)
	if p.defaultMgr != nil {
		_ = p.defaultMgr.client.Close()
	}
	return nil
}
