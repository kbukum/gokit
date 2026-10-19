package jetstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/nats"
)

func TestNewRefusesCoreOnlyAndInvalidSettings(t *testing.T) {
	t.Parallel()
	for _, cfg := range []nats.Config{
		{URL: "tls://localhost:4222", SubjectPrefix: "svc"},
		{URL: "tls://localhost:4222", QueueGroup: "workers"},
		{URL: "nats://localhost:4222"},
	} {
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatalf("New(%+v) accepted invalid config", cfg)
		}
	}
}

func TestNewReconnectsWithoutLimitByDefault(t *testing.T) {
	t.Parallel()
	c, err := New(context.Background(), unreachable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if c.cfg.MaxReconnects != -1 {
		t.Fatalf("max reconnects = %d", c.cfg.MaxReconnects)
	}
	bounded := unreachable
	bounded.MaxReconnects = 3
	b, err := New(context.Background(), bounded)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if b.cfg.MaxReconnects != 3 {
		t.Fatalf("bounded max reconnects = %d", b.cfg.MaxReconnects)
	}
}

func TestNewHonorsItsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(ctx, unreachable); !errors.Is(err, context.Canceled) {
		t.Fatalf("New with a canceled context = %v", err)
	}
	d := &dialer{ctx: ctx}
	if _, err := d.Dial("tcp", "127.0.0.1:1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("first dial ignored its context: %v", err)
	}
	d.release()
	if _, err := d.Dial("tcp", "127.0.0.1:1"); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("reconnect dial used the start context: %v", err)
	}
}

func TestDeadConnectionFailsOperationsAndHealth(t *testing.T) {
	t.Parallel()
	c, err := New(context.Background(), unreachable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	// A connection whose reconnect bound ran out closes by itself.
	c.conn.Close()
	if !c.dead() {
		t.Fatal("closed connection not reported dead")
	}
	if _, err := c.api(); !errors.Is(err, natsgo.ErrConnectionClosed) || !permanent(err) {
		t.Fatalf("api on a dead connection = %v", err)
	}
	comp := mustComponent(t, unreachable)
	comp.client = c
	if h := comp.Health(context.Background()); h.Status != component.StatusUnhealthy {
		t.Fatalf("health = %+v", h)
	}
	cons, err := c.Consumer(consumerConfig(func(error) {}))
	if err != nil {
		t.Fatal(err)
	}
	if err := cons.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, natsgo.ErrConnectionClosed) {
		t.Fatalf("consume on a dead connection = %v", err)
	}
}

func TestStreamConfigValidate(t *testing.T) {
	t.Parallel()
	if err := validStream().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*StreamConfig){
		"name":          func(c *StreamConfig) { c.Name = "bad.name" },
		"no subjects":   func(c *StreamConfig) { c.Subjects = nil },
		"bad subject":   func(c *StreamConfig) { c.Subjects = []string{"facts..x"} },
		"mid wildcard":  func(c *StreamConfig) { c.Subjects = []string{"facts.>.x"} },
		"max age":       func(c *StreamConfig) { c.MaxAge = 0 },
		"max bytes":     func(c *StreamConfig) { c.MaxBytes = 0 },
		"msg size":      func(c *StreamConfig) { c.MaxMsgSize = 0 },
		"dup window":    func(c *StreamConfig) { c.Duplicates = 2 * time.Hour },
		"many replicas": func(c *StreamConfig) { c.Replicas = 6 },
	} {
		c := validStream()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestConsumerConfigDefaultsAndValidate(t *testing.T) {
	t.Parallel()
	c := ConsumerConfig{Durable: "usage", FilterSubjects: []string{"facts.*.>"}, OnError: func(error) {}}
	c.ApplyDefaults()
	if c.AckWait != DefaultAckWait || c.MaxAckPending != DefaultMaxAckPending || c.Batch != DefaultBatch ||
		c.FetchWait != DefaultFetchWait || c.RetryDelay != DefaultRetryDelay || c.MaxRetryDelay != DefaultMaxRetryDelay ||
		c.Delivery.MaxAttempts != 0 {
		t.Fatalf("defaults = %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	small := ConsumerConfig{Stream: "FACTS", Durable: "usage", MaxAckPending: 5, RetryDelay: time.Minute, OnError: c.OnError}
	small.ApplyDefaults()
	if small.Batch != 5 || small.MaxRetryDelay != time.Minute || small.Validate() != nil {
		t.Fatalf("small defaults = %+v", small)
	}
	for name, mutate := range map[string]func(*ConsumerConfig){
		"durable":       func(c *ConsumerConfig) { c.Durable = "u.x" },
		"no observer":   func(c *ConsumerConfig) { c.OnError = nil },
		"stream":        func(c *ConsumerConfig) { c.Stream = "a b" },
		"no target":     func(c *ConsumerConfig) { c.FilterSubjects = nil },
		"filter":        func(c *ConsumerConfig) { c.FilterSubjects = []string{"a..b"} },
		"ack wait":      func(c *ConsumerConfig) { c.AckWait = -1 },
		"attempts":      func(c *ConsumerConfig) { c.Delivery.MaxAttempts = -1 },
		"batch":         func(c *ConsumerConfig) { c.Batch = MaxFetchBatch + 1 },
		"batch>pending": func(c *ConsumerConfig) { c.Batch = c.MaxAckPending + 1 },
		"retry":         func(c *ConsumerConfig) { c.MaxRetryDelay = c.RetryDelay - 1 },
	} {
		bad := c
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	if (ConsumerConfig{Stream: "FACTS"}).topic() != "FACTS" || c.topic() != "facts.*.>" {
		t.Fatal("topic naming")
	}
}

func TestDeliveryPolicyNeedsAnExplicitChoiceToGiveUp(t *testing.T) {
	t.Parallel()
	hook := func(context.Context, messaging.Message, error) error { return nil }
	for name, p := range map[string]DeliveryPolicy{
		"retry until success": {},
		"terminal hand-off":   {OnDeadLetter: hook},
		"bounded hand-off":    {MaxAttempts: 3, OnDeadLetter: hook},
		"bounded discard":     {MaxAttempts: 3, DiscardExhausted: true},
	} {
		if err := p.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, p := range map[string]DeliveryPolicy{
		"negative":        {MaxAttempts: -1},
		"silent drop":     {MaxAttempts: 3},
		"both choices":    {MaxAttempts: 3, OnDeadLetter: hook, DiscardExhausted: true},
		"nothing to drop": {DiscardExhausted: true},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	if err := (DeliveryPolicy{MaxAttempts: 3}).Validate(); !strings.Contains(err.Error(), "on_dead_letter") || !strings.Contains(err.Error(), "discard_exhausted") {
		t.Fatalf("error does not explain the options: %v", err)
	}
}

func TestProducerConfigValidate(t *testing.T) {
	t.Parallel()
	for _, cfg := range []ProducerConfig{{Stream: "a.b"}, {RetryAttempts: -1}, {RetryBackoff: -1}} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%+v) accepted invalid config", cfg)
		}
	}
}

func TestEnsureStreamMapping(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	if err := testClient(fake).EnsureStream(context.Background(), validStream()); err != nil {
		t.Fatal(err)
	}
	s := fake.stream
	if s.Storage != natsjs.FileStorage || s.Retention != natsjs.LimitsPolicy || s.Discard != natsjs.DiscardOld ||
		s.Replicas != 1 || s.Duplicates != time.Minute || s.MaxMsgSize != 1024 {
		t.Fatalf("stream config = %+v", s)
	}
	bad := validStream()
	bad.MaxAge = 0
	if err := testClient(fake).EnsureStream(context.Background(), bad); err == nil {
		t.Fatal("invalid stream accepted")
	}
}

func TestClientCloseIsIdempotentAndRefusesUse(t *testing.T) {
	t.Parallel()
	c := testClient(&fakeJS{})
	p, err := c.Producer(ProducerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.Connected() {
		t.Fatal("closed client reports connected")
	}
	if err := p.PublishBinary(context.Background(), "facts.a", "", nil); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("publish after close = %v", err)
	}
	if err := c.EnsureStream(context.Background(), validStream()); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("ensure after close = %v", err)
	}
	if _, err := c.Replay(context.Background(), ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: 1, Wait: time.Second}); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("replay after close = %v", err)
	}
}
