package middleware

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/messaging"
)

// DedupConfig configures the deduplication middleware.
type DedupConfig struct {
	// KeyFunc extracts the dedup key from a message.
	// Default: message "message-id" header.
	KeyFunc func(messaging.Message) string

	// WindowSize is the maximum number of entries in the dedup cache.
	// Oldest entries are evicted when the limit is reached.
	// Default: 10000.
	WindowSize int

	// TTL is how long an entry is retained before it is considered expired.
	// Default: 5 minutes.
	TTL time.Duration
}

func (c *DedupConfig) applyDefaults() {
	if c.KeyFunc == nil {
		c.KeyFunc = func(msg messaging.Message) string {
			return msg.Headers["message-id"]
		}
	}
	if c.WindowSize <= 0 {
		c.WindowSize = 10000
	}
	if c.TTL <= 0 {
		c.TTL = 5 * time.Minute
	}
}

type dedupEntry struct {
	key  string
	seen time.Time
}

// dedupCache is a bounded LRU cache with TTL support.
type dedupCache struct {
	mu      sync.Mutex
	entries *list.List
	index   map[string]*list.Element
	maxSize int
	ttl     time.Duration
	nowFunc func() time.Time // for testing
}

func newDedupCache(maxSize int, ttl time.Duration) *dedupCache {
	return &dedupCache{
		entries: list.New(),
		index:   make(map[string]*list.Element, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
		nowFunc: time.Now,
	}
}

// seen returns true if key is already in the cache (and not expired).
// If absent or expired, it adds/refreshes the entry and returns false.
func (c *dedupCache) seen(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.nowFunc()

	if elem, ok := c.index[key]; ok {
		entry := elem.Value.(*dedupEntry)
		if now.Sub(entry.seen) < c.ttl {
			// Move to front (most recently seen).
			c.entries.MoveToFront(elem)
			entry.seen = now
			return true
		}
		// Expired — remove and re-add below.
		c.entries.Remove(elem)
		delete(c.index, key)
	}

	// Evict oldest if at capacity.
	for c.entries.Len() >= c.maxSize {
		back := c.entries.Back()
		if back == nil {
			break
		}
		old := c.entries.Remove(back).(*dedupEntry)
		delete(c.index, old.key)
	}

	elem := c.entries.PushFront(&dedupEntry{key: key, seen: now})
	c.index[key] = elem
	return false
}

// DedupHandler wraps a MessageHandler with deduplication logic.
// Messages with a previously seen key (within the TTL window) are silently
// skipped. This follows the existing middleware convention in gokit.
func DedupHandler(handler messaging.MessageHandler, cfg DedupConfig) messaging.MessageHandler {
	cfg.applyDefaults()
	cache := newDedupCache(cfg.WindowSize, cfg.TTL)

	return func(ctx context.Context, msg messaging.Message) error {
		key := cfg.KeyFunc(msg)
		if key == "" {
			// No dedup key — always process.
			return handler(ctx, msg)
		}
		if cache.seen(key) {
			return nil // duplicate — skip
		}
		return handler(ctx, msg)
	}
}
