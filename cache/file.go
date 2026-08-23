package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// defaultMaxEntryBytes bounds a single serialized file-cache entry (16 MiB),
// matching the cross-kit file-cache default and protecting against unbounded reads.
const defaultMaxEntryBytes int64 = 16 * 1024 * 1024

// FileConfig configures the filesystem cache backend.
type FileConfig struct {
	// Root is the directory that holds cache entries. It is created on demand.
	Root string `mapstructure:"root" json:"root" yaml:"root"`

	// KeyPrefix namespaces keys so independent caches may share a Root without collisions.
	KeyPrefix string `mapstructure:"key_prefix" json:"key_prefix" yaml:"key_prefix"`

	// MaxEntryBytes rejects serialized entries larger than this bound. Zero selects
	// the 16 MiB default.
	MaxEntryBytes int64 `mapstructure:"max_entry_bytes" json:"max_entry_bytes" yaml:"max_entry_bytes"`

	// DefaultTTL is applied when Set is called with ttl == 0. A resulting zero
	// duration means the entry never expires.
	DefaultTTL time.Duration `mapstructure:"default_ttl" json:"default_ttl" yaml:"default_ttl"`
}

// FileStore is a persistent cache backed by sharded files under a confined root.
// Entries are addressed by a content hash of their prefixed key, so keys never map
// onto filesystem paths directly and directory traversal is impossible. Writes are
// serialized and atomic (temp file + rename); reads run concurrently.
type FileStore struct {
	root          string
	keyPrefix     string
	maxEntryBytes int64
	defaultTTL    time.Duration
	clock         func() time.Time
	mu            sync.Mutex
}

// fileEntry is the on-disk representation of a cache entry. Value is base64-encoded
// by the JSON codec, preserving arbitrary bytes.
type fileEntry struct {
	Key       string    `json:"key"`
	Value     []byte    `json:"value"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// NewFileStore creates a filesystem cache rooted at cfg.Root, creating the root
// directory if necessary. Root must be non-empty.
func NewFileStore(cfg FileConfig) (*FileStore, error) {
	store := newFileStore(cfg, time.Now)
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func newFileStore(cfg FileConfig, clock func() time.Time) *FileStore {
	maxEntry := cfg.MaxEntryBytes
	if maxEntry <= 0 {
		maxEntry = defaultMaxEntryBytes
	}
	return &FileStore{
		root:          cfg.Root,
		keyPrefix:     cfg.KeyPrefix,
		maxEntryBytes: maxEntry,
		defaultTTL:    cfg.DefaultTTL,
		clock:         clock,
	}
}

func (s *FileStore) init() error {
	if s.root == "" {
		return fmt.Errorf("cache: file store requires a non-empty root")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("cache: create file store root: %w", err)
	}
	return nil
}

// RegisterFile registers the filesystem backend into an explicit registry.
func RegisterFile(reg *FactoryRegistry) error {
	return reg.Register(ProviderFile, func(cfg Config, providerCfg any, _ *logging.Logger) (Store, error) {
		fileCfg := FileConfig{KeyPrefix: cfg.Name, DefaultTTL: cfg.DefaultTTL}
		if providerCfg != nil {
			pc, ok := providerCfg.(*FileConfig)
			if !ok {
				return nil, &ConfigTypeError{Provider: ProviderFile, Expected: "*cache.FileConfig", Actual: providerCfg}
			}
			fileCfg = *pc
			if fileCfg.DefaultTTL == 0 {
				fileCfg.DefaultTTL = cfg.DefaultTTL
			}
		}
		return NewFileStore(fileCfg)
	})
}

// Get returns a cached value when present and unexpired. Expired entries are
// reported as a miss but left in place for CleanupExpired to reclaim.
func (s *FileStore) Get(ctx context.Context, key string) (value []byte, found bool, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, false, ctxErr
	}
	path := s.entryPath(key)
	blob, err := readBounded(path, s.maxEntryBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var entry fileEntry
	if err := json.Unmarshal(blob, &entry); err != nil {
		return nil, false, fmt.Errorf("cache: decode file entry: %w", err)
	}
	if entry.Key != s.prefixedKey(key) {
		return nil, false, fmt.Errorf("cache: file entry key mismatch (hash collision) for %q", key)
	}
	if !entry.ExpiresAt.IsZero() && !s.clock().Before(entry.ExpiresAt) {
		return nil, false, nil
	}
	return entry.Value, true, nil
}

// Set writes value for key with the given TTL. ttl == 0 adopts the store default;
// a resulting zero duration means the entry never expires.
func (s *FileStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl == 0 {
		ttl = s.defaultTTL
	}
	entry := fileEntry{Key: s.prefixedKey(key), Value: value}
	if ttl > 0 {
		entry.ExpiresAt = s.clock().Add(ttl)
	}
	blob, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache: encode file entry: %w", err)
	}
	if int64(len(blob)) > s.maxEntryBytes {
		return fmt.Errorf("cache: entry size %d exceeds max_entry_bytes %d", len(blob), s.maxEntryBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.entryPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("cache: create shard directory: %w", err)
	}
	return atomicWrite(path, blob)
}

// Delete removes key. A missing key is not an error.
func (s *FileStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.entryPath(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cache: delete file entry: %w", err)
	}
	return nil
}

// Exists reports whether key is present and unexpired.
func (s *FileStore) Exists(ctx context.Context, key string) (bool, error) {
	_, ok, err := s.Get(ctx, key)
	return ok, err
}

// CleanupExpired scans at most maxEntries files and deletes expired entries,
// returning the number removed. It is application-invoked maintenance; the store
// performs no automatic capacity eviction.
func (s *FileStore) CleanupExpired(ctx context.Context, maxEntries int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock()
	removed, scanned := 0, 0
	walkErr := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if scanned >= maxEntries {
			return filepath.SkipAll
		}
		scanned++
		blob, readErr := readBounded(path, s.maxEntryBytes)
		if readErr != nil {
			return nil //nolint:nilerr // best-effort cleanup skips unreadable/oversized files
		}
		var entry fileEntry
		if json.Unmarshal(blob, &entry) != nil {
			return nil //nolint:nilerr // best-effort cleanup skips undecodable files
		}
		if !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) {
			if os.Remove(path) == nil { //nolint:gosec // path is enumerated from within the confined root
				removed++
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
		return removed, fmt.Errorf("cache: cleanup expired: %w", walkErr)
	}
	return removed, nil
}

func (s *FileStore) prefixedKey(key string) string {
	if s.keyPrefix == "" {
		return key
	}
	return s.keyPrefix + ":" + key
}

func (s *FileStore) entryPath(key string) string {
	hash := util.HashHex([]byte(s.prefixedKey(key)))
	return filepath.Join(s.root, hash[:2], hash)
}

// readBounded reads the file at path but refuses entries larger than limit bytes,
// stat-checking the size before reading so a tampered or corrupt oversized file
// can never be pulled fully into memory. The final read goes through an
// io.LimitReader as a second guard against a size that grows between stat and read.
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // path derives from a hashed key confined to root
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cache: stat file entry: %w", err)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("cache: file entry size %d exceeds max_entry_bytes %d", info.Size(), limit)
	}
	blob, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("cache: read file entry: %w", err)
	}
	if int64(len(blob)) > limit {
		return nil, fmt.Errorf("cache: file entry exceeds max_entry_bytes %d", limit)
	}
	return blob, nil
}

func atomicWrite(path string, blob []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("cache: create temp entry: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: write temp entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: close temp entry: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: commit entry: %w", err)
	}
	return nil
}

var _ Store = (*FileStore)(nil)
