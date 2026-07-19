package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/messaging"
)

type fakeNATSConn struct {
	published  []*natsgo.Msg
	sub        natsSubscription
	closed     bool
	publishErr error
	flushErr   error
	drainErr   error
}

func (c *fakeNATSConn) PublishMsg(msg *natsgo.Msg) error {
	if c.publishErr != nil {
		return c.publishErr
	}
	c.published = append(c.published, msg)
	return nil
}
func (c *fakeNATSConn) FlushTimeout(time.Duration) error               { return c.flushErr }
func (c *fakeNATSConn) Drain() error                                   { return c.drainErr }
func (c *fakeNATSConn) Close()                                         { c.closed = true }
func (c *fakeNATSConn) IsClosed() bool                                 { return c.closed }
func (c *fakeNATSConn) SubscribeSync(string) (natsSubscription, error) { return c.sub, nil }
func (c *fakeNATSConn) QueueSubscribeSync(string, string) (natsSubscription, error) {
	return c.sub, nil
}

func TestProducerPublishMethodsUseSubjectHeadersAndPayloads(t *testing.T) {
	conn := &fakeNATSConn{}
	p := &Producer{cfg: Config{SubjectPrefix: "svc", PublishTimeout: "1s"}, conn: conn, retryAttempts: 1}
	ctx := context.Background()

	if err := p.PublishJSON(ctx, "orders", "json-key", map[string]string{"id": "1"}); err != nil {
		t.Fatalf("PublishJSON: %v", err)
	}
	if err := p.PublishBinary(ctx, "orders", "bin-key", []byte("raw")); err != nil {
		t.Fatalf("PublishBinary: %v", err)
	}
	event := messaging.Event{ID: "evt-1", Type: "created", Source: "test", Subject: "subject-key", Timestamp: time.Unix(1, 0)}
	if err := p.Publish(ctx, "events", event); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := p.Send(ctx, messaging.Message{Topic: "raw", Key: "msg-key", Value: []byte("body"), Headers: map[string]string{"x": "y"}}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(conn.published) != 4 {
		t.Fatalf("published %d messages, want 4", len(conn.published))
	}
	if got := conn.published[0].Subject; got != "svc.orders" {
		t.Fatalf("subject = %q, want svc.orders", got)
	}
	if got := conn.published[0].Header.Get("message-key"); got != "json-key" {
		t.Fatalf("json message-key = %q", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal(conn.published[0].Data, &decoded); err != nil || decoded["id"] != "1" {
		t.Fatalf("json payload = %#v, err %v", decoded, err)
	}
	if got := conn.published[1].Header.Get("content-type"); got != "application/octet-stream" {
		t.Fatalf("binary content-type = %q", got)
	}
	if got := conn.published[2].Header.Get("event-id"); got != "evt-1" {
		t.Fatalf("event-id = %q", got)
	}
	if got := conn.published[3].Header.Get("x"); got != "y" {
		t.Fatalf("send header = %q", got)
	}
}

func TestProducerSendBatchStopsOnErrorAndCloseDrains(t *testing.T) {
	conn := &fakeNATSConn{}
	p := &Producer{cfg: Config{PublishTimeout: "1s"}, conn: conn, retryAttempts: 1}
	err := p.SendBatch(context.Background(), []messaging.Message{{Topic: "a", Value: []byte("1")}, {Topic: "bad topic"}})
	if err == nil {
		t.Fatal("expected invalid topic error")
	}
	if len(conn.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(conn.published))
	}
	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !conn.closed {
		t.Fatal("Close should close connection")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestProducerMapsPublishFlushAndContextErrors(t *testing.T) {
	publishErr := errors.New("publish failed")
	p := &Producer{cfg: Config{PublishTimeout: "1s"}, conn: &fakeNATSConn{publishErr: publishErr}, retryAttempts: 1}
	if err := p.PublishBinary(context.Background(), "orders", "", []byte("x")); !errors.Is(err, publishErr) {
		t.Fatalf("publish error = %v, want %v", err, publishErr)
	}

	flushErr := errors.New("flush failed")
	p = &Producer{cfg: Config{PublishTimeout: "1s"}, conn: &fakeNATSConn{flushErr: flushErr}, retryAttempts: 1}
	if err := p.PublishBinary(context.Background(), "orders", "", []byte("x")); !errors.Is(err, flushErr) {
		t.Fatalf("flush error = %v, want %v", err, flushErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p = &Producer{cfg: Config{PublishTimeout: "1s"}, conn: &fakeNATSConn{}, retryAttempts: 1}
	if err := p.PublishBinary(ctx, "orders", "", []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled publish = %v, want context.Canceled", err)
	}

	p.closed = true
	if err := p.Flush(context.Background()); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("closed flush = %v, want ErrClosed", err)
	}
}
