package nats

import (
	"context"
	"errors"
	"testing"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/messaging"
)

type fakeNATSSub struct {
	msgs chan *natsgo.Msg
	err  error
	unsubErr error
	unsubscribed bool
}

func (s *fakeNATSSub) NextMsgWithContext(ctx context.Context) (*natsgo.Msg, error) {
	if s.err != nil {
		return nil, s.err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-s.msgs:
		if !ok {
			return nil, errors.New("closed")
		}
		return msg, nil
	}
}
func (s *fakeNATSSub) Unsubscribe() error { s.unsubscribed = true; return s.unsubErr }

func TestConsumerConsumeDeliversHeadersAndStopsOnContext(t *testing.T) {
	sub := &fakeNATSSub{msgs: make(chan *natsgo.Msg, 1)}
	sub.msgs <- &natsgo.Msg{Subject: "svc.orders", Data: []byte("payload"), Header: natsgo.Header{"message-key": {"k"}, "x": {"y"}}}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Consumer{sub: sub, topic: "orders"}
	var got messaging.Message
	err := c.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
		got = msg
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v, want context.Canceled", err)
	}
	if got.Topic != "orders" || got.Key != "k" || string(got.Value) != "payload" || got.Headers["x"] != "y" {
		t.Fatalf("delivered message = %#v", got)
	}
}

func TestConsumerHandlerAndReceiveErrors(t *testing.T) {
	handlerErr := errors.New("handler failed")
	sub := &fakeNATSSub{msgs: make(chan *natsgo.Msg, 1)}
	sub.msgs <- &natsgo.Msg{Data: []byte("payload")}
	c := &Consumer{sub: sub, topic: "orders"}
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return handlerErr }); !errors.Is(err, handlerErr) {
		t.Fatalf("handler error = %v, want %v", err, handlerErr)
	}

	receiveErr := errors.New("receive failed")
	c = &Consumer{sub: &fakeNATSSub{err: receiveErr}, topic: "orders"}
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, receiveErr) {
		t.Fatalf("receive error = %v, want %v", err, receiveErr)
	}
	c.closed = true
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("closed receive error = %v, want ErrClosed", err)
	}
}

func TestConsumerEnsureSubscriptionAndClose(t *testing.T) {
	original := connectNATS
	defer func() { connectNATS = original }()
	sub := &fakeNATSSub{msgs: make(chan *natsgo.Msg)}
	conn := &fakeNATSConn{sub: sub}
	connectNATS = func(string, ...natsgo.Option) (natsConn, error) { return conn, nil }

	c, err := NewConsumer(Config{URL: "nats://localhost:4222", AllowInsecureDev: true, QueueGroup: "workers"}, "orders")
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if c.Topic() != "orders" {
		t.Fatalf("Topic = %q", c.Topic())
	}
	if got, err := c.ensureSubscription(); err != nil || got != sub {
		t.Fatalf("ensureSubscription = %v, %v", got, err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !sub.unsubscribed || !conn.closed {
		t.Fatalf("Close unsubscribed=%v closed=%v", sub.unsubscribed, conn.closed)
	}
	if _, err := c.ensureSubscription(); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("closed ensure = %v, want ErrClosed", err)
	}
}

func FuzzSubject(f *testing.F) {
	f.Add("svc", "orders")
	f.Add(".svc.", "events.created")
	f.Fuzz(func(t *testing.T, prefix, topic string) {
		cfg := Config{SubjectPrefix: prefix}
		got := subject(cfg, topic)
		if strings.Trim(prefix, ".") == "" && got != topic {
			t.Fatalf("subject without prefix = %q, want %q", got, topic)
		}
	})
}
