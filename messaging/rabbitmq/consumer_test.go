package rabbitmq

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kbukum/gokit/messaging"
)

type fakeAcknowledger struct{ acked, nacked int }

func (a *fakeAcknowledger) Ack(uint64, bool) error        { a.acked++; return nil }
func (a *fakeAcknowledger) Nack(uint64, bool, bool) error { a.nacked++; return nil }
func (a *fakeAcknowledger) Reject(uint64, bool) error     { return nil }

func TestConsumerConsumeAcksSuccessfulMessages(t *testing.T) {
	acks := &fakeAcknowledger{}
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{Acknowledger: acks, DeliveryTag: 1, Body: []byte("payload"), Timestamp: time.Unix(1, 0), Headers: amqp.Table{"message-key": "k", "x": "y"}}
	ch := &fakeRabbitChannel{deliveries: deliveries}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Consumer{ch: ch, cfg: Config{PrefetchCount: 2}, topic: "orders", queue: "orders"}
	var got messaging.Message
	err := c.Consume(ctx, func(_ context.Context, msg messaging.Message) error { got = msg; cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v", err)
	}
	if acks.acked != 1 || acks.nacked != 0 {
		t.Fatalf("acks=%d nacks=%d", acks.acked, acks.nacked)
	}
	if got.Topic != "orders" || got.Key != "k" || string(got.Value) != "payload" || got.Headers["x"] != "y" {
		t.Fatalf("message = %#v", got)
	}
}

func TestConsumerNacksHandlerErrorAndMapsConsumeErrors(t *testing.T) {
	acks := &fakeAcknowledger{}
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{Acknowledger: acks, DeliveryTag: 1, Body: []byte("payload")}
	ch := &fakeRabbitChannel{deliveries: deliveries}
	handlerErr := errors.New("handler failed")
	c := &Consumer{ch: ch, topic: "orders", queue: "orders"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return handlerErr }); !errors.Is(err, handlerErr) {
		t.Fatalf("handler error = %v", err)
	}
	if acks.nacked != 1 {
		t.Fatalf("nacks=%d, want 1", acks.nacked)
	}

	consumeErr := errors.New("consume failed")
	c = &Consumer{ch: &fakeRabbitChannel{consumeErr: consumeErr}, topic: "orders", queue: "orders"}
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, consumeErr) {
		t.Fatalf("consume error = %v", err)
	}
	c.closed = true
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("closed consume error = %v", err)
	}
}

func TestConsumerEnsureChannelAndClose(t *testing.T) {
	original := dialRabbit
	defer func() { dialRabbit = original }()
	ch := &fakeRabbitChannel{deliveries: make(chan amqp.Delivery)}
	conn := &fakeRabbitConn{ch: ch}
	dialRabbit = func(Config) (rabbitConn, error) { return conn, nil }
	c, err := NewConsumer(Config{URL: "amqp://localhost:5672/", AllowInsecureDev: true, QueuePrefix: "q"}, "orders")
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if c.Topic() != "orders" {
		t.Fatalf("Topic = %q", c.Topic())
	}
	if got, err := c.ensureChannel(); err != nil || got != ch {
		t.Fatalf("ensureChannel = %v, %v", got, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !ch.closed || !conn.closed {
		t.Fatalf("closed ch=%v conn=%v", ch.closed, conn.closed)
	}
	if _, err := c.ensureChannel(); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("closed ensure = %v", err)
	}
}

func FuzzRoutingKeyAndQueueName(f *testing.F) {
	f.Add("rk", "q", "orders")
	f.Add(".rk.", "", "events.created")
	f.Fuzz(func(t *testing.T, rkPrefix, qPrefix, topic string) {
		cfg := Config{RoutingKeyPrefix: rkPrefix, QueuePrefix: qPrefix}
		route := routingKey(cfg, topic)
		if strings.Trim(rkPrefix, ".") == "" && route != topic {
			t.Fatalf("route = %q, want topic", route)
		}
		queue := queueName(cfg, topic)
		if strings.Trim(qPrefix, ".") == "" && queue != route {
			t.Fatalf("queue = %q, want route %q", queue, route)
		}
	})
}
