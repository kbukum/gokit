package jetstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/nats"
)

// unreachable points at a closed loopback port; the client keeps reconnecting in the background.
var unreachable = nats.Config{URL: "nats://127.0.0.1:1", AllowInsecureDev: true, ReconnectWait: "1h"}

func TestComponentLifecycle(t *testing.T) {
	t.Parallel()
	c := NewComponent("broker", unreachable)
	ctx := context.Background()
	if c.Name() != "broker" || c.Describe().Details != "nats://127.0.0.1:1" {
		t.Fatalf("name=%q describe=%+v", c.Name(), c.Describe())
	}
	p, err := c.Producer(ProducerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cons, err := c.Consumer(ConsumerConfig{Stream: "FACTS", Durable: "usage", OnError: func(error) {}})
	if err != nil {
		t.Fatal(err)
	}
	notStarted := func(err error) bool {
		var app *apperrors.AppError
		return errors.As(err, &app) && app.Code == apperrors.ErrCodeServiceUnavailable
	}
	if err := p.PublishBinary(ctx, "facts.a", "", nil); !notStarted(err) {
		t.Fatalf("publish before start = %v", err)
	}
	if err := cons.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); !notStarted(err) {
		t.Fatalf("consume before start = %v", err)
	}
	if err := c.EnsureStream(ctx, validStream()); !notStarted(err) {
		t.Fatalf("ensure before start = %v", err)
	}
	if _, err := c.Replay(ctx, ReplayRequest{}); !notStarted(err) {
		t.Fatalf("replay before start = %v", err)
	}
	if h := c.Health(ctx); h.Status != component.StatusDegraded {
		t.Fatalf("health before start = %+v", h)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if h := c.Health(ctx); h.Status != component.StatusDegraded || h.Message != "broker unreachable" {
		t.Fatalf("health while unreachable = %+v", h)
	}
	if _, err := c.Replay(ctx, ReplayRequest{}); err == nil || notStarted(err) {
		t.Fatalf("replay after start should reach validation, got %v", err)
	}
	if err := c.EnsureStream(ctx, StreamConfig{}); err == nil || notStarted(err) {
		t.Fatalf("ensure after start should reach validation, got %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishBinary(ctx, "facts.a", "", nil); !notStarted(err) {
		t.Fatalf("publish after stop = %v", err)
	}
}

func TestComponentRefusesInvalidConfigAndCanceledStart(t *testing.T) {
	t.Parallel()
	if err := NewComponent("broker", nats.Config{URL: "nats://localhost:4222"}).Start(context.Background()); err == nil {
		t.Fatal("cleartext URL accepted without allow_insecure_dev")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewComponent("broker", unreachable).Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled start = %v", err)
	}
}

func common(mutate func(*messaging.Config)) messaging.Config {
	cfg := messaging.Config{
		Adapter: AdapterName, DeliveryGuarantee: messaging.DeliveryAtLeastOnce, CommitStrategy: messaging.CommitAfterHandlerSuccess,
		ConsumerGroup: "usage", RequestTimeout: "1s",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func TestRegisterBuildsOwningProducerAndConsumer(t *testing.T) {
	t.Parallel()
	reg := messaging.NewRegistry()
	if err := Register(reg, unreachable); err != nil {
		t.Fatal(err)
	}
	if err := Register(reg, unreachable); err == nil {
		t.Fatal("double registration accepted")
	}
	ctx := context.Background()
	p, err := reg.NewProducer(ctx, common(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	jp, ok := p.(*Producer)
	if !ok || jp.owned == nil || jp.owned.publishTimeout() != time.Second {
		t.Fatalf("producer = %T", p)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := reg.NewConsumer(ctx, common(nil), nil, "facts.a")
	if err != nil {
		t.Fatal(err)
	}
	jc, ok := c.(*Consumer)
	if !ok || jc.owned == nil || !strings.HasPrefix(jc.cfg.Durable, "usage-") || c.Topic() != "facts.a" {
		t.Fatalf("consumer = %+v", c)
	}
	// Each topic in a group keeps its own durable consumer; the same topic shares one.
	same, err := reg.NewConsumer(ctx, common(nil), nil, "facts.a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := reg.NewConsumer(ctx, common(nil), nil, "facts.b")
	if err != nil {
		t.Fatal(err)
	}
	if same.(*Consumer).cfg.Durable != jc.cfg.Durable || other.(*Consumer).cfg.Durable == jc.cfg.Durable {
		t.Fatalf("durables: %s %s %s", jc.cfg.Durable, same.(*Consumer).cfg.Durable, other.(*Consumer).cfg.Durable)
	}
	for _, x := range []messaging.Consumer{same, other} {
		if err := x.Close(); err != nil {
			t.Fatal(err)
		}
	}
	jc.cfg.OnError(errTransient)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterRefusesUnsupportedSemantics(t *testing.T) {
	t.Parallel()
	if err := Register(nil, unreachable); err == nil {
		t.Fatal("nil registry accepted")
	}
	if err := Register(messaging.NewRegistry(), nats.Config{URL: "tls://x:1", SubjectPrefix: "svc"}); err == nil {
		t.Fatal("core-only settings accepted")
	}
	reg := messaging.NewRegistry()
	if err := Register(reg, unreachable); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, mutate := range map[string]func(*messaging.Config){
		"at most once": func(c *messaging.Config) { c.DeliveryGuarantee = messaging.DeliveryAtMostOnce },
		"auto commit":  func(c *messaging.Config) { c.CommitStrategy = messaging.CommitAuto },
		"in flight":    func(c *messaging.Config) { c.MaxInFlight = 4 },
		"dlq":          func(c *messaging.Config) { c.DLQ.Enabled = true },
	} {
		if _, err := reg.NewProducer(ctx, common(mutate), nil); err == nil {
			t.Fatalf("%s: producer accepted", name)
		}
		if _, err := reg.NewConsumer(ctx, common(mutate), nil, "facts.a"); err == nil {
			t.Fatalf("%s: consumer accepted", name)
		}
	}
	if _, err := reg.NewConsumer(ctx, common(func(c *messaging.Config) { c.ConsumerGroup = "" }), nil, "facts.a"); err == nil {
		t.Fatal("consumer without a durable name accepted")
	}
	if _, err := reg.NewConsumer(ctx, common(func(c *messaging.Config) { c.ConsumerGroup = "a.b" }), nil, "facts.a"); err == nil {
		t.Fatal("durable name with a dot accepted")
	}
	if _, err := reg.NewConsumer(ctx, common(func(c *messaging.Config) { c.ConsumerGroup = strings.Repeat("g", 48) }), nil, "facts.a"); err == nil {
		t.Fatal("group too long for a durable name accepted")
	}
}
