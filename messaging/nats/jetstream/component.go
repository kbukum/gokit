package jetstream

import (
	"context"
	"sync"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/messaging/nats"
)

// Component owns a shared [Client] for a component registry: Start connects, Stop drains and closes, and Health is
// degraded while the connection is down, so a service keeps serving while its client reconnects. Health is unhealthy
// once a bounded reconnect gives up. Producers and consumers made from it
// before Start resolve the client at each operation and fail with a service-unavailable error until it is started.
type Component struct {
	name   string
	cfg    nats.Config
	mu     sync.RWMutex
	client *Client
}

var (
	_ component.Component   = (*Component)(nil)
	_ component.Describable = (*Component)(nil)
)

// NewComponent returns an unstarted component named name.
func NewComponent(name string, cfg nats.Config) *Component {
	cfg.ApplyDefaults()
	return &Component{name: name, cfg: cfg}
}

// Name returns the component name.
func (c *Component) Name() string { return c.name }

// Start validates the configuration and starts the connection; ctx bounds the first attempt. It does not wait for
// the server.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return nil
	}
	client, err := New(ctx, c.cfg)
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

// Stop drains and closes the client within ctx.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	client := c.client
	c.client = nil
	c.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Shutdown(ctx)
}

// Health is healthy while connected, degraded while the client reconnects or before it starts, and unhealthy once the
// connection gave up reconnecting.
func (c *Component) Health(context.Context) component.Health {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	switch {
	case client == nil:
		return component.Degraded(c.name, "not started")
	case client.dead():
		return component.Unhealthy(c.name, "broker connection closed")
	case !client.Connected():
		return component.Degraded(c.name, "broker unreachable")
	default:
		return component.Healthy(c.name)
	}
}

// Describe summarizes the component for the startup display.
func (c *Component) Describe() component.Description {
	return component.Description{Name: "NATS JetStream", Type: "jetstream", Details: c.cfg.RedactedURL()}
}

// Producer returns a producer over the component's client.
func (c *Component) Producer(cfg ProducerConfig) (*Producer, error) {
	return newProducer(c.started, cfg, nil)
}

// Consumer returns a consumer over the component's client.
func (c *Component) Consumer(cfg ConsumerConfig) (*Consumer, error) {
	return newConsumer(c.started, cfg, nil)
}

// EnsureStream creates the stream or updates it to cfg.
func (c *Component) EnsureStream(ctx context.Context, cfg StreamConfig) error {
	client, err := c.started()
	if err != nil {
		return err
	}
	return client.EnsureStream(ctx, cfg)
}

// Replay reads one page of retained messages; see [Client.Replay].
func (c *Component) Replay(ctx context.Context, r ReplayRequest) (ReplayPage, error) {
	client, err := c.started()
	if err != nil {
		return ReplayPage{}, err
	}
	return client.Replay(ctx, r)
}

func (c *Component) started() (*Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.client == nil {
		return nil, apperrors.ServiceUnavailable(c.name).WithReason("BROKER_NOT_STARTED")
	}
	return c.client, nil
}
