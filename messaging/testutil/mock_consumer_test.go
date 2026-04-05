package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
)

func TestChannelConsumer_Interfaces(t *testing.T) {
	t.Parallel()
	var _ messaging.Consumer = (*ChannelConsumer)(nil)
}

func TestChannelConsumer_FeedAndConsume(t *testing.T) {
	t.Parallel()

	c := NewChannelConsumer("events")

	var received []messaging.Message
	handler := func(_ context.Context, msg messaging.Message) error {
		received = append(received, msg)
		if msg.Key == "fail" {
			return context.DeadlineExceeded
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(ctx, handler)
	}()

	c.Feed(
		messaging.Message{Topic: "events", Key: "k1", Value: []byte("hello")},
		messaging.Message{Topic: "events", Key: "k2", Value: []byte("world")},
	)

	// Give the consumer time to process
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if len(received) != 2 {
		t.Fatalf("received %d messages, want 2", len(received))
	}
	if string(received[0].Value) != "hello" {
		t.Errorf("received[0].Value = %q, want hello", string(received[0].Value))
	}
	if string(received[1].Value) != "world" {
		t.Errorf("received[1].Value = %q, want world", string(received[1].Value))
	}
}

func TestChannelConsumer_Topic(t *testing.T) {
	t.Parallel()
	c := NewChannelConsumer("my-topic")
	if c.Topic() != "my-topic" {
		t.Errorf("Topic() = %q, want my-topic", c.Topic())
	}
}

func TestChannelConsumer_Close(t *testing.T) {
	t.Parallel()
	c := NewChannelConsumer("t")
	if err := c.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestChannelConsumer_CustomBufferSize(t *testing.T) {
	t.Parallel()
	c := NewChannelConsumer("t", 5)
	if c.Topic() != "t" {
		t.Errorf("Topic() = %q, want t", c.Topic())
	}
}
