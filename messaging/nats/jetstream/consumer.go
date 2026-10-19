package jetstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

// Consumer reads a durable pull consumer with explicit acknowledgement. By default Consume declares the durable consumer, creating or updating it, so its position survives restarts and is shared by every process that uses the same durable name; every such process must declare it alike. With ConsumerConfig.Bind it only binds to a durable consumer that something else declared.
type Consumer struct {
	source source
	cfg    ConsumerConfig
	owned  io.Closer
	done   chan struct{}
	mu     sync.Mutex
	closed bool
	// status is nil while fetches succeed, otherwise why the consumer is not receiving.
	status atomic.Pointer[error]
}

var (
	_ messaging.Consumer       = (*Consumer)(nil)
	_ messaging.ConsumerStatus = (*Consumer)(nil)
)

func newConsumer(src source, cfg ConsumerConfig, owned io.Closer) (*Consumer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	c := &Consumer{source: src, cfg: cfg, owned: owned, done: make(chan struct{})}
	c.setStatus(ErrNotReceiving)
	return c, nil
}

// ConsumeStatus is nil once a fetch succeeds, [ErrNotReceiving] before then, and the last broker failure while the loop retries it.
func (c *Consumer) ConsumeStatus() error {
	if err := c.status.Load(); err != nil {
		return *err
	}
	return nil
}

func (c *Consumer) setStatus(err error) {
	if err == nil {
		c.status.Store(nil)
		return
	}
	c.status.Store(&err)
}

// Consume fetches bounded batches and calls handler for each message until ctx ends or the consumer is closed, and returns why: ctx's error, or [messaging.ErrClosed]. Messages are first delivered in stream order; a retried message may come after later ones.
//
// A nil handler result is acknowledged and confirmed by the server. An error naks the message with a delay that doubles per delivery up to MaxRetryDelay, and the loop goes on. An error wrapping [messaging.ErrTerminal], or a failure on the message's last attempt under [DeliveryPolicy], hands it to OnDeadLetter or gives it up as the policy says; a failed hand-off is retried on the next delivery. Each message is marked in progress just before its handler runs, which restarts its AckWait.
//
// Broker failures are reported to OnError and retried with backoff, so the loop survives outages. A fetch the server does not complete by its deadline, such as a pull request a missing permission drops, is such a failure ([ErrFetchIncomplete]); [Consumer.ConsumeStatus] reports it until a fetch completes. A failure that cannot pass, such as a consumer declaration the server refuses or a connection that gave up reconnecting, ends Consume with that error. A missing permission on a request usually shows as a timeout, since the server drops the request without a reply. When Consume stops, fetched messages it has not handled are naked for prompt redelivery.
//
// A message's Topic is its subject, Offset its stream sequence and Timestamp the time the stream stored it.
func (c *Consumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	if handler == nil {
		return errors.New("jetstream: consume handler is required")
	}
	if _, err := c.client(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	err := c.consume(ctx, handler)
	if c.isClosed() {
		return messaging.ErrClosed
	}
	return err
}

func (c *Consumer) consume(ctx context.Context, handler messaging.MessageHandler) error {
	var (
		pull  natsjs.Consumer
		bound *Client
	)
	failures := 0
	fail := func(err error) error {
		failures++
		c.setStatus(err)
		c.cfg.OnError(err)
		return sleep(ctx, resilience.CalculateBackoff(failures, c.cfg.RetryDelay, c.cfg.MaxRetryDelay))
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		client, err := c.client()
		if err != nil {
			return err
		}
		js, err := client.api()
		if err != nil {
			return err
		}
		// A restarted component hands out a new client, so the consumer binds again over its connection.
		if pull == nil || bound != client {
			if pull, err = c.bind(ctx, client, js); err != nil {
				pull = nil
				if permanent(err) {
					return err
				}
				if sleepErr := fail(err); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			bound = client
		}
		fetchCtx, cancel := context.WithTimeout(ctx, c.cfg.FetchWait)
		batch, err := pull.Fetch(c.cfg.Batch, natsjs.FetchContext(fetchCtx))
		if err == nil {
			for msg := range batch.Messages() {
				// A delivered message shows the server answers this consumer's fetches.
				c.setStatus(nil)
				if ctx.Err() != nil {
					c.release(msg)
					continue
				}
				c.deliver(ctx, client, msg, handler)
			}
			err = batch.Error()
		}
		cancel()
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err == nil || errors.Is(err, natsjs.ErrNoMessages):
			failures = 0
			c.setStatus(nil)
		case errors.Is(err, context.DeadlineExceeded):
			// The server ends an idle fetch before its deadline, so this one never completed; the binding stays.
			if sleepErr := fail(fmt.Errorf("jetstream: fetch %s: %w", c.cfg.Durable, ErrFetchIncomplete)); sleepErr != nil {
				return sleepErr
			}
		default:
			// The durable consumer may be gone, for example after a server restart; declaring it again is idempotent.
			pull = nil
			err = fmt.Errorf("jetstream: fetch %s: %w", c.cfg.Durable, err)
			if permanent(err) {
				return err
			}
			if sleepErr := fail(err); sleepErr != nil {
				return sleepErr
			}
		}
	}
}

func (c *Consumer) bind(ctx context.Context, client *Client, js natsjs.JetStream) (natsjs.Consumer, error) {
	ctx, cancel := client.request(ctx)
	defer cancel()
	stream := c.cfg.Stream
	if stream == "" {
		name, err := js.StreamNameBySubject(ctx, c.cfg.FilterSubjects[0])
		if err != nil {
			return nil, fmt.Errorf("jetstream: find stream for %s: %w", c.cfg.FilterSubjects[0], err)
		}
		stream = name
	}
	if c.cfg.Bind {
		pull, err := js.Consumer(ctx, stream, c.cfg.Durable)
		if err != nil {
			return nil, fmt.Errorf("jetstream: bind consumer %s/%s: %w", stream, c.cfg.Durable, err)
		}
		server := pull.CachedInfo().Config
		if server.AckPolicy != natsjs.AckExplicitPolicy {
			return nil, fmt.Errorf("%w: %s/%s has ack policy %s; it must be explicit", ErrUnsafeBoundConsumer, stream, c.cfg.Durable, server.AckPolicy)
		}
		// A server-side delivery limit would end redelivery before a failed hand-off is retried.
		if server.MaxDeliver > 0 {
			return nil, fmt.Errorf("%w: %s/%s has max_deliver %d; set it to -1 and bound attempts with Delivery.MaxAttempts", ErrUnsafeBoundConsumer, stream, c.cfg.Durable, server.MaxDeliver)
		}
		return pull, nil
	}
	durable := natsjs.ConsumerConfig{
		Durable: c.cfg.Durable, FilterSubjects: c.cfg.FilterSubjects, AckPolicy: natsjs.AckExplicitPolicy,
		DeliverPolicy: natsjs.DeliverAllPolicy, AckWait: c.cfg.AckWait, MaxAckPending: c.cfg.MaxAckPending,
		// The consumer counts attempts itself; an unlimited broker limit keeps a failed hand-off retryable.
		MaxDeliver: -1,
	}
	if c.cfg.Declare != nil {
		if err := client.EnsureStream(ctx, *c.cfg.Declare); err != nil {
			return nil, err
		}
	}
	pull, err := js.CreateOrUpdateConsumer(ctx, stream, durable)
	if err != nil {
		return nil, fmt.Errorf("jetstream: ensure consumer %s/%s: %w", stream, c.cfg.Durable, err)
	}
	return pull, nil
}

func (c *Consumer) deliver(ctx context.Context, client *Client, msg natsjs.Msg, handler messaging.MessageHandler) {
	m, delivered, err := toMessage(msg)
	if err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: metadata: %w", err))
		if err := msg.NakWithDelay(c.cfg.MaxRetryDelay); err != nil {
			c.cfg.OnError(fmt.Errorf("jetstream: nak: %w", err))
		}
		return
	}
	// The message may have waited behind earlier ones in the batch; restart its AckWait before handling it.
	if err := msg.InProgress(); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: in progress %s#%d: %w", m.Topic, m.Offset, err))
	}
	delay := resilience.CalculateBackoff(int(min(delivered, 64)), c.cfg.RetryDelay, c.cfg.MaxRetryDelay)
	if c.cfg.Delivery.spent(delivered) {
		// The last attempt failed and so did its hand-off; retry only the hand-off.
		c.deadLetter(ctx, msg, m, fmt.Errorf("%w: all %d attempts failed", ErrDeliveryLimit, c.cfg.Delivery.MaxAttempts), delay)
		return
	}
	if err := handler(ctx, m); err != nil {
		switch {
		case ctx.Err() != nil:
			c.release(msg)
		case errors.Is(err, messaging.ErrTerminal) || c.cfg.Delivery.exhausted(delivered):
			c.deadLetter(ctx, msg, m, err, delay)
		default:
			c.nak(msg, delay)
		}
		return
	}
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), client.publishTimeout())
	defer cancel()
	if err := msg.DoubleAck(ackCtx); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: ack %s#%d: %w", m.Topic, m.Offset, err))
	}
}

// deadLetter ends redelivery of a message whose handler failed for good. Without a hook, a terminal failure is the handler's explicit choice and is terminated; an exhausted one is terminated only because DiscardExhausted asked for it, and is reported.
func (c *Consumer) deadLetter(ctx context.Context, msg natsjs.Msg, m messaging.Message, cause error, delay time.Duration) {
	hook := c.cfg.Delivery.OnDeadLetter
	if hook == nil {
		if !errors.Is(cause, messaging.ErrTerminal) {
			c.cfg.OnError(fmt.Errorf("%w: %s#%d: %w", ErrDeliveryLimit, m.Topic, m.Offset, cause))
		}
		c.term(msg)
		return
	}
	if err := hook(ctx, m, cause); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: dead letter %s#%d: %w", m.Topic, m.Offset, err))
		c.nak(msg, delay)
		return
	}
	c.term(msg)
}

func (c *Consumer) term(msg natsjs.Msg) {
	if err := msg.Term(); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: term: %w", err))
	}
}

func (c *Consumer) nak(msg natsjs.Msg, delay time.Duration) {
	if err := msg.NakWithDelay(delay); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: nak: %w", err))
	}
}

// release naks a message the stopping loop will not handle, so it is redelivered without waiting for AckWait.
func (c *Consumer) release(msg natsjs.Msg) {
	if err := msg.Nak(); err != nil {
		c.cfg.OnError(fmt.Errorf("jetstream: release: %w", err))
	}
}

// Topic returns the filter subjects, comma-separated, or the stream when the consumer reads all of it.
func (c *Consumer) Topic() string { return c.cfg.topic() }

// Close stops a running Consume at once, including its fetch, backoff and the context its handler sees, and closes the client only if the consumer owns it. The durable consumer and its position remain on the server. It is safe to call more than once.
func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)
	if c.owned != nil {
		return c.owned.Close()
	}
	return nil
}

func (c *Consumer) client() (*Client, error) {
	if c.isClosed() {
		return nil, messaging.ErrClosed
	}
	return c.source()
}

func (c *Consumer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// toMessage maps a stored message and returns its delivery count.
func toMessage(msg natsjs.Msg) (messaging.Message, uint64, error) {
	meta, err := msg.Metadata()
	if err != nil {
		return messaging.Message{}, 0, err
	}
	headers := make(map[string]string, len(msg.Headers()))
	for k, v := range msg.Headers() {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return messaging.Message{
		Key: headers[keyHeader], Payload: msg.Data(), Topic: msg.Subject(), Offset: offset(meta.Sequence.Stream),
		Timestamp: meta.Timestamp, Headers: headers,
	}, meta.NumDelivered, nil
}

func offset(seq uint64) int64 {
	if seq > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(seq)
}
