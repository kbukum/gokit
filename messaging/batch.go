package messaging

import (
	"context"
	"sync"
	"time"
)

// BatchConfig configures a BatchProducer.
type BatchConfig struct {
	MaxSize  int           // Max messages per batch (default: 100).
	MaxWait  time.Duration // Max time before forced flush (default: 5s).
	MaxBytes int64         // Max total bytes per batch (0 = unlimited).
}

func (c *BatchConfig) applyDefaults() {
	if c.MaxSize <= 0 {
		c.MaxSize = 100
	}
	if c.MaxWait <= 0 {
		c.MaxWait = 5 * time.Second
	}
}

// BatchProducer buffers messages and flushes them in batches via an
// underlying Producer. It is safe for concurrent use.
type BatchProducer struct {
	producer Producer
	topic    string
	cfg      BatchConfig

	mu       sync.Mutex
	buf      []Message
	bufBytes int64
	timer    *time.Timer
	done     chan struct{}
	closed   bool
}

// NewBatchProducer creates a BatchProducer that publishes to topic via p.
func NewBatchProducer(p Producer, topic string, cfg BatchConfig) *BatchProducer {
	cfg.applyDefaults()
	b := &BatchProducer{
		producer: p,
		topic:    topic,
		cfg:      cfg,
		buf:      make([]Message, 0, cfg.MaxSize),
		done:     make(chan struct{}),
	}
	b.timer = time.AfterFunc(cfg.MaxWait, b.timerFlush)
	return b
}

// Send buffers a message. A flush is triggered automatically when MaxSize
// or MaxBytes limits are reached.
func (b *BatchProducer) Send(ctx context.Context, msg Message) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return context.Canceled
	}

	b.buf = append(b.buf, msg)
	b.bufBytes += int64(len(msg.Value))

	needsFlush := len(b.buf) >= b.cfg.MaxSize ||
		(b.cfg.MaxBytes > 0 && b.bufBytes >= b.cfg.MaxBytes)

	if !needsFlush {
		b.mu.Unlock()
		return nil
	}

	batch := b.drainLocked()
	b.mu.Unlock()

	return b.publishBatch(ctx, batch)
}

// Flush forces sending all buffered messages immediately.
func (b *BatchProducer) Flush(ctx context.Context) error {
	b.mu.Lock()
	batch := b.drainLocked()
	b.mu.Unlock()

	return b.publishBatch(ctx, batch)
}

// Close flushes remaining messages and stops the background timer.
func (b *BatchProducer) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.timer.Stop()
	batch := b.drainLocked()
	b.mu.Unlock()

	close(b.done)
	return b.publishBatch(ctx, batch)
}

// drainLocked extracts and resets the buffer. Caller must hold b.mu.
func (b *BatchProducer) drainLocked() []Message {
	if len(b.buf) == 0 {
		return nil
	}
	batch := b.buf
	b.buf = make([]Message, 0, b.cfg.MaxSize)
	b.bufBytes = 0
	b.timer.Reset(b.cfg.MaxWait)
	return batch
}

// timerFlush is called by the background timer.
func (b *BatchProducer) timerFlush() {
	b.mu.Lock()
	batch := b.drainLocked()
	b.mu.Unlock()

	if len(batch) > 0 {
		// Use background context because the timer fires outside any request.
		_ = b.publishBatch(context.Background(), batch)
	}
}

// publishBatch sends each message in the batch via the underlying Producer.
func (b *BatchProducer) publishBatch(ctx context.Context, batch []Message) error {
	for _, msg := range batch {
		if err := b.producer.PublishBinary(ctx, b.topic, msg.Key, msg.Value); err != nil {
			return err
		}
	}
	return nil
}
