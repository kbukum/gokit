package memory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/messaging"
)

// InMemoryConsumer implements messaging.Consumer using an InMemoryBroker.
type InMemoryConsumer struct {
	broker         *InMemoryBroker
	topic          string
	ch             chan messaging.Message
	commitStrategy messaging.CommitStrategy
	done           chan struct{}
	closed         bool               // protected by broker.mu
	pending        *messaging.Message // one failed delivery, protected by broker.mu
	consuming      atomic.Bool
}

var _ messaging.Consumer = (*InMemoryConsumer)(nil)

// Consume blocks reading from the broker channel, calling handler for each message.
func (c *InMemoryConsumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	if !c.consuming.CompareAndSwap(false, true) {
		return apperrors.New(apperrors.ErrCodeConflict, "consumer is already running")
	}
	defer c.consuming.Store(false)
	for {
		msg, err := c.next(ctx)
		if err != nil {
			return err
		}
		if err := handler(ctx, msg); err != nil {
			if c.commitStrategy == messaging.CommitAfterHandlerSuccess {
				return errors.Join(err, c.retain(msg))
			}
			return err
		}
	}
}

func (c *InMemoryConsumer) next(ctx context.Context) (messaging.Message, error) {
	if err := ctx.Err(); err != nil {
		return messaging.Message{}, err
	}
	c.broker.mu.Lock()
	if c.closed || c.broker.closed {
		c.broker.mu.Unlock()
		return messaging.Message{}, messaging.ErrClosed
	}
	if c.pending != nil {
		message := *c.pending
		c.pending = nil
		c.broker.mu.Unlock()
		return message, nil
	}
	c.broker.mu.Unlock()
	select {
	case <-ctx.Done():
		return messaging.Message{}, ctx.Err()
	case <-c.done:
		return messaging.Message{}, messaging.ErrClosed
	case <-c.broker.closeCh:
		return messaging.Message{}, messaging.ErrClosed
	case message := <-c.ch:
		return message, nil
	}
}

func (c *InMemoryConsumer) retain(message messaging.Message) error {
	c.broker.mu.Lock()
	defer c.broker.mu.Unlock()
	if c.closed || c.broker.closed {
		return fmt.Errorf("retain failed delivery: %w", messaging.ErrClosed)
	}
	c.pending = &message
	return nil
}

// Topic returns the consumer's topic.
func (c *InMemoryConsumer) Topic() string { return c.topic }

// Close releases the subscription and interrupts blocked consumption.
func (c *InMemoryConsumer) Close() error {
	c.broker.mu.Lock()
	defer c.broker.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.pending = nil
	close(c.done)
	subs := slices.DeleteFunc(c.broker.topics[c.topic], func(sub *InMemoryConsumer) bool { return sub == c })
	if len(subs) == 0 {
		delete(c.broker.topics, c.topic)
	} else {
		c.broker.topics[c.topic] = subs
	}
	for {
		select {
		case <-c.ch:
		default:
			return nil
		}
	}
}
