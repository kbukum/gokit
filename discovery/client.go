package discovery

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/skillsenselab/gokit/logger"
)

// ClientConfig configures the discovery Client.
type ClientConfig struct {
	// CacheTTL is how long discovered endpoints are cached. Default: 30s.
	CacheTTL time.Duration

	// Services lists the service names this client will discover.
	Services []string

	// Criticality maps service names to their criticality level.
	// Required services cause errors on discovery failure; optional ones are skipped.
	Criticality map[string]Criticality
}

// Client is a high-level discovery client that adds caching and load balancing
// on top of a Discovery backend.
type Client struct {
	discovery Discovery
	cache     *instanceCache
	cfg       ClientConfig
	log       *logger.Logger
	r         *rand.Rand
	mu        sync.Mutex
	rrIndex   map[string]int
}

// NewClient creates a Client that wraps the given Discovery backend.
func NewClient(disc Discovery, cfg ClientConfig, log *logger.Logger) *Client {
	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = 30 * time.Second
	}
	return &Client{
		discovery: disc,
		cache:     newInstanceCache(ttl),
		cfg:       cfg,
		log:       log,
		r:         rand.New(rand.NewSource(time.Now().UnixNano())),
		rrIndex:   make(map[string]int),
	}
}

// Discover returns all healthy instances of a service, using cache when fresh.
// Optional protocol parameter filters results by protocol tag.
func (c *Client) Discover(ctx context.Context, serviceName string, protocol ...string) ([]ServiceInstance, error) {
	if instances := c.cache.get(serviceName); instances != nil {
		if len(protocol) > 0 && protocol[0] != "" {
			return filterByProtocol(instances, protocol[0]), nil
		}
		return instances, nil
	}

	instances, err := c.discovery.Discover(ctx, serviceName)
	if err != nil {
		crit := c.cfg.Criticality[serviceName]
		if crit == CriticalityRequired {
			return nil, fmt.Errorf("required service %q discovery failed: %w", serviceName, err)
		}
		return nil, err
	}

	c.cache.set(serviceName, instances)

	if len(protocol) > 0 && protocol[0] != "" {
		return filterByProtocol(instances, protocol[0]), nil
	}
	return instances, nil
}

// DiscoverOne returns a single instance selected by the query's load-balancing strategy.
func (c *Client) DiscoverOne(ctx context.Context, query Query) (ServiceInstance, error) {
	instances, err := c.Discover(ctx, query.ServiceName)
	if err != nil {
		return ServiceInstance{}, err
	}

	// Filter by protocol if specified.
	if query.Protocol != "" {
		instances = filterByProtocol(instances, query.Protocol)
	}

	if len(instances) == 0 {
		return ServiceInstance{}, ErrNoHealthyEndpoints
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch query.Strategy {
	case StrategyRoundRobin:
		key := query.ServiceName + ":" + query.Protocol
		idx := c.rrIndex[key]
		inst := instances[idx%len(instances)]
		c.rrIndex[key] = (idx + 1) % len(instances)
		return inst, nil

	case StrategyWeighted:
		return c.selectWeighted(instances), nil

	case StrategyRandom:
		fallthrough
	default:
		return instances[c.r.Intn(len(instances))], nil
	}
}

// DiscoverAll returns instances for all configured services.
func (c *Client) DiscoverAll(ctx context.Context) (map[string][]ServiceInstance, error) {
	result := make(map[string][]ServiceInstance)
	for _, name := range c.cfg.Services {
		instances, err := c.Discover(ctx, name)
		if err != nil {
			if c.cfg.Criticality[name] == CriticalityRequired {
				return nil, err
			}
			continue
		}
		result[name] = instances
	}
	return result, nil
}

// Invalidate clears cached entries for a service.
func (c *Client) Invalidate(serviceName string) {
	c.cache.invalidate(serviceName)
}

// Close releases resources.
func (c *Client) Close() error {
	c.cache.clear()
	return c.discovery.Close()
}

func (c *Client) selectWeighted(instances []ServiceInstance) ServiceInstance {
	totalWeight := 0
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	if totalWeight == 0 {
		return instances[c.r.Intn(len(instances))]
	}

	r := c.r.Intn(totalWeight)
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		r -= w
		if r < 0 {
			return inst
		}
	}
	return instances[0]
}

func filterByProtocol(instances []ServiceInstance, protocol string) []ServiceInstance {
	var filtered []ServiceInstance
	protocolLower := strings.ToLower(protocol)
	tag := "protocol:" + protocolLower
	for _, inst := range instances {
		// Match on Protocol field first
		if strings.EqualFold(inst.Protocol, protocol) {
			filtered = append(filtered, inst)
			continue
		}
		// Fall back to tag matching
		for _, t := range inst.Tags {
			tl := strings.ToLower(t)
			if tl == tag || tl == protocolLower {
				filtered = append(filtered, inst)
				break
			}
		}
	}
	return filtered
}

// --- instance cache ---

type instanceCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	instances []ServiceInstance
	expiry    time.Time
}

func newInstanceCache(ttl time.Duration) *instanceCache {
	return &instanceCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *instanceCache) get(serviceName string) []ServiceInstance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[serviceName]
	if !ok || time.Now().After(entry.expiry) {
		return nil
	}
	return entry.instances
}

func (c *instanceCache) set(serviceName string, instances []ServiceInstance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[serviceName] = cacheEntry{
		instances: instances,
		expiry:    time.Now().Add(c.ttl),
	}
}

func (c *instanceCache) invalidate(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if strings.HasPrefix(key, serviceName) {
			delete(c.entries, key)
		}
	}
}

func (c *instanceCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

// Compile-time check that Client implements DiscoveryClient.
var _ DiscoveryClient = (*Client)(nil)
