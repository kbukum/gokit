package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kbukum/gokit/messaging"
)

type fakeRabbitConn struct {
	ch rabbitChannel
	closed bool
	channelErr error
	closeErr error
}

func (c *fakeRabbitConn) Channel() (rabbitChannel, error) {
	if c.channelErr != nil { return nil, c.channelErr }
	return c.ch, nil
}
func (c *fakeRabbitConn) Close() error { c.closed = true; return c.closeErr }

type fakeRabbitChannel struct {
	declaredQueues []string
	boundKeys []string
	published []rabbitPublish
	deliveries chan amqp.Delivery
	closed bool
	queueErr error
	bindErr error
	publishErr error
	consumeErr error
	qosErr error
	closeErr error
}

type rabbitPublish struct { exchange, key string; msg amqp.Publishing }

func (ch *fakeRabbitChannel) ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error { return nil }
func (ch *fakeRabbitChannel) QueueDeclare(name string, _ bool, _ bool, _ bool, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if ch.queueErr != nil { return amqp.Queue{}, ch.queueErr }
	ch.declaredQueues = append(ch.declaredQueues, name)
	return amqp.Queue{Name: name}, nil
}
func (ch *fakeRabbitChannel) QueueBind(_ string, key string, _ string, _ bool, _ amqp.Table) error {
	if ch.bindErr != nil { return ch.bindErr }
	ch.boundKeys = append(ch.boundKeys, key)
	return nil
}
func (ch *fakeRabbitChannel) PublishWithContext(ctx context.Context, exchange, key string, _ bool, _ bool, msg amqp.Publishing) error {
	if err := ctx.Err(); err != nil { return err }
	if ch.publishErr != nil { return ch.publishErr }
	ch.published = append(ch.published, rabbitPublish{exchange: exchange, key: key, msg: msg})
	return nil
}
func (ch *fakeRabbitChannel) Qos(int, int, bool) error { return ch.qosErr }
func (ch *fakeRabbitChannel) ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error) {
	if ch.consumeErr != nil { return nil, ch.consumeErr }
	return ch.deliveries, nil
}
func (ch *fakeRabbitChannel) Close() error { ch.closed = true; return ch.closeErr }

func TestProducerPublishMethodsDeclareQueueAndPublish(t *testing.T) {
	ch := &fakeRabbitChannel{}
	p := &Producer{cfg: Config{Exchange: "ex", RoutingKeyPrefix: "rk", QueuePrefix: "q", PublishTimeout: "1s"}, ch: ch, retryAttempts: 1, declared: map[string]struct{}{}}
	ctx := context.Background()
	if err := p.PublishJSON(ctx, "orders", "json-key", map[string]string{"id": "1"}); err != nil { t.Fatalf("PublishJSON: %v", err) }
	if err := p.PublishBinary(ctx, "orders", "bin-key", []byte("raw")); err != nil { t.Fatalf("PublishBinary: %v", err) }
	event := messaging.Event{ID: "evt-1", Type: "created", Source: "test", Subject: "subject-key", Timestamp: time.Unix(1, 0)}
	if err := p.Publish(ctx, "events", event); err != nil { t.Fatalf("Publish: %v", err) }
	if err := p.Send(ctx, messaging.Message{Topic: "raw", Key: "msg-key", Value: []byte("body"), Headers: map[string]string{"x": "y"}}); err != nil { t.Fatalf("Send: %v", err) }
	if len(ch.published) != 4 { t.Fatalf("published %d messages, want 4", len(ch.published)) }
	if got := ch.published[0].key; got != "rk.orders" { t.Fatalf("routing key = %q", got) }
	if got := ch.published[0].msg.Headers["message-key"]; got != "json-key" { t.Fatalf("message-key = %v", got) }
	var decoded map[string]string
	if err := json.Unmarshal(ch.published[0].msg.Body, &decoded); err != nil || decoded["id"] != "1" { t.Fatalf("json payload = %#v err %v", decoded, err) }
	if len(ch.declaredQueues) != 3 { t.Fatalf("declared queues = %v, want one per topic", ch.declaredQueues) }
	if got := ch.published[2].msg.Headers["event-id"]; got != "evt-1" { t.Fatalf("event-id = %v", got) }
	if got := ch.published[3].msg.Headers["x"]; got != "y" { t.Fatalf("send header = %v", got) }
}

func TestProducerErrorsFlushAndClose(t *testing.T) {
	publishErr := errors.New("publish failed")
	p := &Producer{cfg: Config{PublishTimeout: "1s"}, ch: &fakeRabbitChannel{publishErr: publishErr}, retryAttempts: 1, declared: map[string]struct{}{}}
	if err := p.PublishBinary(context.Background(), "orders", "", []byte("x")); !errors.Is(err, publishErr) { t.Fatalf("publish error = %v", err) }
	ctx, cancel := context.WithCancel(context.Background()); cancel()
	if err := p.Flush(ctx); !errors.Is(err, context.Canceled) { t.Fatalf("Flush = %v", err) }
	ch := &fakeRabbitChannel{}
	conn := &fakeRabbitConn{ch: ch}
	p = &Producer{ch: ch, conn: conn, declared: map[string]struct{}{}}
	if err := p.Close(); err != nil { t.Fatalf("Close: %v", err) }
	if !ch.closed || !conn.closed { t.Fatalf("closed ch=%v conn=%v", ch.closed, conn.closed) }
	if err := p.Close(); err != nil { t.Fatalf("second Close: %v", err) }
}

func TestProducerSendBatchStopsOnError(t *testing.T) {
	ch := &fakeRabbitChannel{}
	p := &Producer{cfg: Config{PublishTimeout: "1s"}, ch: ch, retryAttempts: 1, declared: map[string]struct{}{}}
	err := p.SendBatch(context.Background(), []messaging.Message{{Topic: "a", Value: []byte("1")}, {Topic: "bad topic"}})
	if err == nil { t.Fatal("expected invalid topic error") }
	if len(ch.published) != 1 { t.Fatalf("published %d, want 1", len(ch.published)) }
}
