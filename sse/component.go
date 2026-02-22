package sse

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/component"
)

// Component wraps an SSE Hub as a lifecycle-managed component.
// Register it with the component registry so Start/Stop are handled automatically.
type Component struct {
	hub  *Hub
	wg   sync.WaitGroup
	mu   sync.Mutex
	path string
}

// ensure Component satisfies component.Component and Describable.
var (
	_ component.Component   = (*Component)(nil)
	_ component.Describable = (*Component)(nil)
)

// NewComponent creates a new SSE component with a fresh Hub.
func NewComponent(path string) *Component {
	return &Component{
		hub:  NewHub(),
		path: path,
	}
}

// Hub returns the underlying Hub for event broadcasting and client management.
func (c *Component) Hub() *Hub { return c.hub }

// Name returns the component name.
func (c *Component) Name() string { return "sse" }

// Start launches the Hub's event loop in a background goroutine.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.hub.Run()
	}()

	return nil
}

// Stop signals the Hub to shut down and waits for Run to return.
func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hub.Stop()
	c.wg.Wait()
	return nil
}

// Health returns the health status of the SSE hub.
func (c *Component) Health(_ context.Context) component.Health {
	return component.Health{
		Name:    c.Name(),
		Status:  component.StatusHealthy,
		Message: fmt.Sprintf("%d clients connected", c.hub.GetClientCount()),
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	return component.Description{
		Name:    "SSE Hub",
		Type:    "sse",
		Details: fmt.Sprintf("Path: %s", c.path),
	}
}
