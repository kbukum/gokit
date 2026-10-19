package jetstream

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/nats"
)

// Client owns one JetStream connection, which reconnects in the background, including when the server is not yet reachable at construction. Producers and consumers made from it share the connection; Close releases it.
//
// Subjects are absolute: nats.Config.SubjectPrefix and QueueGroup apply only to core NATS and are refused. A zero nats.Config.MaxReconnects means unlimited here, so a broker outage delays work rather than ending it; a positive bound lets the connection die, after which every operation fails and [Component] reports Unhealthy.
type Client struct {
	cfg    nats.Config
	conn   *natsgo.Conn
	js     natsjs.JetStream
	done   chan struct{}
	mu     sync.Mutex
	closed bool
}

// source resolves the client a producer or consumer uses at the time of each operation.
type source func() (*Client, error)

// New validates cfg and starts a connection. ctx bounds only the first connection attempt; New does not wait for the server, and operations fail with a connection error until it is reachable.
func New(ctx context.Context, cfg nats.Config) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = -1
	}
	if err := validateClientConfig(cfg); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	var once sync.Once
	dial := &dialer{ctx: ctx, net: net.Dialer{Timeout: mustDuration(cfg.ConnectTimeout)}}
	conn, err := nats.Connect(cfg, natsgo.RetryOnFailedConnect(true), natsgo.SetCustomDialer(dial),
		natsgo.ClosedHandler(func(*natsgo.Conn) { once.Do(func() { close(done) }) }))
	dial.release()
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		conn.Close()
		return nil, ctxErr
	}
	js, err := natsjs.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	return &Client{cfg: cfg, conn: conn, js: js, done: done}, nil
}

// dialer bounds the first connection attempt by the caller's context; reconnects dial without it.
type dialer struct {
	mu  sync.Mutex
	ctx context.Context
	net net.Dialer
}

func (d *dialer) Dial(network, address string) (net.Conn, error) {
	d.mu.Lock()
	ctx := d.ctx
	d.mu.Unlock()
	return d.net.DialContext(ctx, network, address)
}

func (d *dialer) release() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ctx = context.Background()
}

func validateClientConfig(cfg nats.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.SubjectPrefix != "" || cfg.QueueGroup != "" {
		return errors.New("jetstream: subject_prefix and queue_group apply only to core NATS")
	}
	return nil
}

// EnsureStream creates the stream or updates it to cfg.
func (c *Client) EnsureStream(ctx context.Context, cfg StreamConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	js, err := c.api()
	if err != nil {
		return err
	}
	ctx, cancel := c.request(ctx)
	defer cancel()
	_, err = js.CreateOrUpdateStream(ctx, natsjs.StreamConfig{
		Name: cfg.Name, Subjects: cfg.Subjects, Storage: natsjs.FileStorage, Retention: natsjs.LimitsPolicy,
		Discard: natsjs.DiscardOld, MaxAge: cfg.MaxAge, MaxBytes: cfg.MaxBytes, MaxMsgSize: cfg.MaxMsgSize,
		Duplicates: cfg.Duplicates, Replicas: max(cfg.Replicas, 1), MaxMsgs: -1, MaxConsumers: -1, MaxMsgsPerSubject: -1,
	})
	if err != nil {
		return fmt.Errorf("jetstream: ensure stream %s: %w", cfg.Name, err)
	}
	return nil
}

// Producer returns a producer that publishes over this client. Closing it leaves the client open.
func (c *Client) Producer(cfg ProducerConfig) (*Producer, error) {
	return newProducer(c.self, cfg, nil)
}

// Consumer returns a consumer of the durable consumer cfg declares, over this client. Closing it leaves the client open.
func (c *Client) Consumer(cfg ConsumerConfig) (*Consumer, error) {
	return newConsumer(c.self, cfg, nil)
}

// Connected reports whether the client is open and its connection is up.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.conn != nil && c.conn.IsConnected()
}

// dead reports that the connection closed by itself, after its reconnect bound ran out.
func (c *Client) dead() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.conn != nil && c.conn.IsClosed()
}

// Close drains the connection, bounded by nats.Config.DrainTimeout, and closes it. It is safe to call more than once.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), mustDuration(c.cfg.DrainTimeout)+cleanupTimeout)
	defer cancel()
	return c.Shutdown(ctx)
}

// Shutdown drains the connection, so pending publications and acknowledgements are flushed, and waits until it is closed or ctx ends, when it closes the connection at once. It is safe to call more than once.
func (c *Client) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn, done := c.conn, c.done
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	if err := conn.Drain(); err != nil {
		conn.Close()
		// A connection that is closed or still reconnecting has nothing to drain.
		if errors.Is(err, natsgo.ErrConnectionClosed) || errors.Is(err, natsgo.ErrConnectionReconnecting) {
			return nil
		}
		return fmt.Errorf("jetstream: drain: %w", err)
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		conn.Close()
		return fmt.Errorf("jetstream: drain: %w", ctx.Err())
	}
}

func (c *Client) self() (*Client, error) { return c, nil }

func (c *Client) api() (natsjs.JetStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, messaging.ErrClosed
	}
	if c.conn != nil && c.conn.IsClosed() {
		return nil, fmt.Errorf("jetstream: reconnect limit reached: %w", natsgo.ErrConnectionClosed)
	}
	return c.js, nil
}

func (c *Client) publishTimeout() time.Duration { return mustDuration(c.cfg.PublishTimeout) }

// request bounds one JetStream API request by nats.Config.PublishTimeout: a request sent during a reconnect or refused by a permission gets no reply, and would otherwise wait for the caller's whole context.
func (c *Client) request(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.publishTimeout())
}

func mustDuration(value string) time.Duration {
	d, _ := time.ParseDuration(value)
	return d
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
