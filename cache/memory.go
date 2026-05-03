package cache

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/logger"
)

// MemoryConfig configures the in-memory cache backend.
type MemoryConfig struct {
	DefaultTTL time.Duration `mapstructure:"default_ttl" json:"default_ttl" yaml:"default_ttl"`
}

// MemoryStore is a thread-safe in-memory cache with TTL expiration.
type MemoryStore struct {
	mu         sync.RWMutex
	clock      func() time.Time
	defaultTTL time.Duration
	items      map[string]memoryItem
}

type memoryItem struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryStore creates an in-memory cache.
func NewMemoryStore(cfg MemoryConfig) *MemoryStore {
	return newMemoryStore(cfg, time.Now)
}

func newMemoryStore(cfg MemoryConfig, clock func() time.Time) *MemoryStore {
	return &MemoryStore{
		clock:      clock,
		defaultTTL: cfg.DefaultTTL,
		items:      make(map[string]memoryItem),
	}
}

// RegisterMemory registers the core memory backend into an explicit registry.
func RegisterMemory(reg *FactoryRegistry) error {
	return reg.Register(ProviderMemory, func(cfg Config, providerCfg any, _ *logger.Logger) (Store, error) {
		memCfg := MemoryConfig{DefaultTTL: cfg.DefaultTTL}
		if providerCfg != nil {
			pc, ok := providerCfg.(*MemoryConfig)
			if !ok {
				return nil, &ConfigTypeError{Provider: ProviderMemory, Expected: "*cache.MemoryConfig", Actual: providerCfg}
			}
			memCfg = *pc
		}
		if memCfg.DefaultTTL == 0 {
			memCfg.DefaultTTL = cfg.DefaultTTL
		}
		return NewMemoryStore(memCfg), nil
	})
}

// Get returns a copy of the cached bytes when present and not expired.
func (s *MemoryStore) Get(ctx context.Context, key string) (value []byte, found bool, err error) {
	s.mu.RLock()
	item, ok := s.items[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if item.expired(s.clock()) {
		_ = s.Delete(ctx, key)
		return nil, false, nil
	}
	return cloneBytes(item.value), true, nil
}

// Set stores a copy of value with the given TTL. ttl=0 uses the store default;
// a resulting zero TTL means no expiration.
func (s *MemoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = s.defaultTTL
	}
	item := memoryItem{value: cloneBytes(value)}
	if ttl > 0 {
		item.expiresAt = s.clock().Add(ttl)
	}
	s.mu.Lock()
	s.items[key] = item
	s.mu.Unlock()
	return nil
}

// Delete removes a key.
func (s *MemoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
	return nil
}

// Exists reports whether key is present and unexpired.
func (s *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	_, ok, err := s.Get(ctx, key)
	return ok, err
}

// GetMany returns present, unexpired keys.
func (s *MemoryStore) GetMany(ctx context.Context, keys []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(keys))
	for _, key := range keys {
		value, ok, err := s.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if ok {
			out[key] = value
		}
	}
	return out, nil
}

func (i memoryItem) expired(now time.Time) bool {
	return !i.expiresAt.IsZero() && !now.Before(i.expiresAt)
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

var (
	_ Store      = (*MemoryStore)(nil)
	_ BatchStore = (*MemoryStore)(nil)
)
