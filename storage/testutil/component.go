package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/testutil"
)

// memFile holds a stored object's data and metadata.
type memFile struct {
	data        []byte
	contentType string
	modTime     time.Time
}

// Component is a test storage component backed by an in-memory map.
// It implements component.Component, testutil.TestComponent, and storage.Storage.
type Component struct {
	files   map[string]*memFile
	started bool
	mu      sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)
var _ storage.Storage = (*Component)(nil)

// NewComponent creates a new in-memory storage test component.
func NewComponent() *Component {
	return &Component{}
}

// Storage returns the component itself as a storage.Storage interface.
func (c *Component) Storage() storage.Storage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return nil
	}
	return c
}

// --- component.Component ---

func (c *Component) Name() string { return "storage-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	c.files = make(map[string]*memFile)
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = nil
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	c.files = make(map[string]*memFile)
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return nil, fmt.Errorf("component not started")
	}
	snap := make(map[string]*memFile, len(c.files))
	for k, v := range c.files {
		cp := *v
		cp.data = append([]byte(nil), v.data...)
		snap[k] = &cp
	}
	return snap, nil
}

func (c *Component) Restore(_ context.Context, snap interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	s, ok := snap.(map[string]*memFile)
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected map[string]*memFile, got %T", snap)
	}
	c.files = make(map[string]*memFile, len(s))
	for k, v := range s {
		cp := *v
		cp.data = append([]byte(nil), v.data...)
		c.files[k] = &cp
	}
	return nil
}

// --- storage.Storage ---

func (c *Component) Upload(_ context.Context, path string, reader io.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read upload data: %w", err)
	}
	c.files[path] = &memFile{data: data, modTime: time.Now()}
	return nil
}

func (c *Component) Download(_ context.Context, path string) (io.ReadCloser, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

func (c *Component) Delete(_ context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.files, path)
	return nil
}

func (c *Component) Exists(_ context.Context, path string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.files[path]
	return ok, nil
}

func (c *Component) URL(_ context.Context, path string) (string, error) {
	return "mem://" + path, nil
}

func (c *Component) List(_ context.Context, prefix string) ([]storage.FileInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var result []storage.FileInfo
	for path, f := range c.files {
		if strings.HasPrefix(path, prefix) {
			result = append(result, storage.FileInfo{
				Path:         path,
				Size:         int64(len(f.data)),
				LastModified: f.modTime,
				ContentType:  f.contentType,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
