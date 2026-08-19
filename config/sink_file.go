package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// maxSinkFileBytes bounds the config sink file size accepted on read (1 MiB). A
// larger file is rejected rather than buffered unbounded.
const maxSinkFileBytes int64 = 1024 * 1024

// sinkTempPrefix is the temp-file prefix used for atomic replacement.
const sinkTempPrefix = "config"

// FileConfigSink is a ConfigSink that persists keys to a flat table on disk. The
// on-disk representation is pluggable via codec.Codec; TOML is the default. Reads are
// bounded and writes are atomic through the fs package, so a concurrent reader never
// observes a partial write. Mutations are read-modify-write sequences serialized by a
// shared lock, so concurrent writers never lose each other's updates. Persisting writes
// the plaintext value to disk: this is the sink's explicit, intended persistence, so
// protect the file with appropriate permissions; the plaintext is never logged.
type FileConfigSink struct {
	path  string
	codec codec.Codec
	mu    *sync.Mutex
}

// NewFileConfigSink creates a file sink backed by path using the TOML codec. The file
// is created on first write; a missing file reads as empty.
func NewFileConfigSink(path string) *FileConfigSink {
	return &FileConfigSink{path: path, codec: codec.NewTOMLCodec(), mu: &sync.Mutex{}}
}

// NewFileConfigSinkWithCodec creates a file sink backed by path with an explicit
// codec, so the table can be persisted as JSON, YAML, or any supported format.
func NewFileConfigSinkWithCodec(path string, c codec.Codec) *FileConfigSink {
	return &FileConfigSink{path: path, codec: c, mu: &sync.Mutex{}}
}

// Path returns the backing file path.
func (s *FileConfigSink) Path() string { return s.path }

// Set stores or replaces the value at key, persisting the updated table atomically.
func (s *FileConfigSink) Set(key string, value SecretString) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	table, err := s.readTable()
	if err != nil {
		return err
	}
	table[key] = value.Value()
	return s.writeTable(table)
}

// Remove deletes the value at key; removing an absent key succeeds without a write.
func (s *FileConfigSink) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	table, err := s.readTable()
	if err != nil {
		return err
	}
	if _, ok := table[key]; !ok {
		return nil
	}
	delete(table, key)
	return s.writeTable(table)
}

// SetMany applies all entries under a single lock and one atomic write.
func (s *FileConfigSink) SetMany(entries []ConfigEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	table, err := s.readTable()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		table[entry.Key] = entry.Value.Value()
	}
	return s.writeTable(table)
}

func (s *FileConfigSink) readTable() (map[string]string, error) {
	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, apperrors.InvalidInput("config",
			fmt.Sprintf("failed to inspect config file '%s'", s.path)).WithCause(err)
	}
	contents, err := fs.ReadFileLimit(s.path, maxSinkFileBytes)
	if err != nil {
		return nil, err
	}
	value, err := s.codec.DecodeValue(string(contents))
	if err != nil {
		return nil, apperrors.InvalidInput("config",
			fmt.Sprintf("failed to parse config file '%s'", s.path)).WithCause(err)
	}
	table, ok := valueIntoTable(value)
	if !ok {
		return nil, apperrors.InvalidInput("config",
			fmt.Sprintf("config file '%s' must be a flat table of string values", s.path))
	}
	return table, nil
}

func (s *FileConfigSink) writeTable(table map[string]string) error {
	value := tableIntoValue(table)
	rendered, err := s.codec.EncodeValue(value)
	if err != nil {
		return apperrors.InvalidInput("config",
			fmt.Sprintf("failed to encode config file '%s'", s.path)).WithCause(err)
	}
	return fs.WriteAtomicReplace(s.path, []byte(rendered), sinkTempPrefix)
}

func tableIntoValue(table map[string]string) codec.Value {
	object := make(map[string]any, len(table))
	for key, value := range table {
		object[key] = value
	}
	return object
}

func valueIntoTable(value codec.Value) (map[string]string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	table := make(map[string]string, len(object))
	for key, entry := range object {
		str, ok := entry.(string)
		if !ok {
			return nil, false
		}
		table[key] = str
	}
	return table, true
}
