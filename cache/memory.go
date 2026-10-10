package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// MemoryConfig configures the in-memory cache backend.
type MemoryConfig struct {
	DefaultTTL time.Duration `mapstructure:"default_ttl" json:"default_ttl" yaml:"default_ttl"`
	// MaxEntries bounds how many keys the store holds. Zero means unbounded. Adding a new key to a full store evicts
	// the least recently used entry; replacing an existing key never evicts.
	MaxEntries int `mapstructure:"max_entries" json:"max_entries" yaml:"max_entries"`
	// Clock supplies time for expiry (default util.SystemClock). Tests inject a fake.
	Clock util.Clock `mapstructure:"-" json:"-" yaml:"-"`
}

// Validate rejects a negative default TTL or bound.
func (c MemoryConfig) Validate() error {
	if c.DefaultTTL < 0 {
		return fmt.Errorf("cache: memory default_ttl must be >= 0")
	}
	if c.MaxEntries < 0 {
		return fmt.Errorf("cache: memory max_entries must be >= 0")
	}
	return nil
}

// MemoryStore is a thread-safe in-memory cache with TTL expiration and an optional least-recently-used bound.
type MemoryStore struct {
	mu         sync.Mutex
	clock      util.Clock
	defaultTTL time.Duration
	maxEntries int
	items      map[string]*list.Element
	order      *list.List // front is the most recently used
}

type memoryItem struct {
	key       string
	value     []byte
	expiresAt time.Time
}

// NewMemoryStore validates cfg and creates an in-memory cache.
func NewMemoryStore(cfg MemoryConfig) (*MemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	clock := cfg.Clock
	if util.IsNil(clock) {
		clock = util.SystemClock{}
	}
	return &MemoryStore{
		clock:      clock,
		defaultTTL: cfg.DefaultTTL,
		maxEntries: cfg.MaxEntries,
		items:      make(map[string]*list.Element),
		order:      list.New(),
	}, nil
}

// RegisterMemory registers the core memory backend into an explicit registry.
func RegisterMemory(reg *FactoryRegistry) error {
	return reg.Register(ProviderMemory, func(cfg Config, providerCfg any, _ *logging.Logger) (Store, error) {
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
		return NewMemoryStore(memCfg)
	})
}

// Get returns a copy of the cached bytes when present and not expired, and marks the key recently used. An expired
// entry is removed.
func (s *MemoryStore) Get(_ context.Context, key string) (value []byte, found bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[key]
	if !ok {
		return nil, false, nil
	}
	item := el.Value.(*memoryItem)
	if item.expired(s.clock.Now()) {
		s.remove(el)
		return nil, false, nil
	}
	s.order.MoveToFront(el)
	return cloneBytes(item.value), true, nil
}

// Set stores a copy of value with the given TTL. ttl=0 uses the store default;
// a resulting zero TTL means no expiration.
func (s *MemoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = s.defaultTTL
	}
	item := &memoryItem{key: key, value: cloneBytes(value)}
	if ttl > 0 {
		item.expiresAt = s.clock.Now().Add(ttl)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[key]; ok {
		el.Value = item
		s.order.MoveToFront(el)
		return nil
	}
	s.items[key] = s.order.PushFront(item)
	if s.maxEntries > 0 && s.order.Len() > s.maxEntries {
		s.remove(s.order.Back())
	}
	return nil
}

// Len reports how many entries the store holds, including expired ones not yet removed.
func (s *MemoryStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}

func (s *MemoryStore) remove(el *list.Element) {
	s.order.Remove(el)
	delete(s.items, el.Value.(*memoryItem).key)
}

// Delete removes a key.
func (s *MemoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[key]; ok {
		s.remove(el)
	}
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

func (i *memoryItem) expired(now time.Time) bool {
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
