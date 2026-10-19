//go:build integration

package testutil_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
	gokitnats "github.com/kbukum/gokit/messaging/nats"
	"github.com/kbukum/gokit/messaging/nats/jetstream"
)

func deliveryConfig(t *testing.T, policy jetstream.DeliveryPolicy) jetstream.ConsumerConfig {
	t.Helper()
	return jetstream.ConsumerConfig{
		Stream: "FACTS", Durable: "usage", FilterSubjects: []string{"facts.>"}, AckWait: time.Second, MaxAckPending: 10, Batch: 5,
		FetchWait: 200 * time.Millisecond, RetryDelay: time.Millisecond, MaxRetryDelay: 5 * time.Millisecond,
		OnError: func(err error) { t.Log(err) }, Delivery: policy,
	}
}

// rawConsumer reads the durable consumer's server-side state with a separate connection.
func rawConsumer(t *testing.T, cfg gokitnats.Config) natsjs.JetStream {
	t.Helper()
	conn, err := gokitnats.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	js, err := natsjs.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	return js
}

func consumerInfo(t *testing.T, js natsjs.JetStream) *natsjs.ConsumerInfo {
	t.Helper()
	c, err := js.Consumer(t.Context(), "FACTS", "usage")
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// waitSettled waits until the server has no pending or unacknowledged message for the consumer.
func waitSettled(t *testing.T, js natsjs.JetStream) *natsjs.ConsumerInfo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		info := consumerInfo(t, js)
		if info.NumPending == 0 && info.NumAckPending == 0 && info.NumRedelivered == 0 {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("consumer did not settle: %+v", info)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The default policy keeps retrying past the old 20-delivery cap until the handler succeeds.
func TestJetStreamDefaultPolicyRetriesUntilSuccess(t *testing.T) {
	t.Parallel()
	client, cfg := startJetStream(t)
	send(t, producer(t, client), "facts.a", "evt-1")
	c, err := client.Consumer(deliveryConfig(t, jetstream.DeliveryPolicy{}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	attempts := 0
	err = c.Consume(ctx, func(context.Context, messaging.Message) error {
		attempts++
		if attempts <= 25 {
			return errors.New("database down")
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || attempts != 26 {
		t.Fatalf("Consume = %v after %d attempts", err, attempts)
	}
	if info := waitSettled(t, rawConsumer(t, cfg)); info.Config.MaxDeliver != -1 {
		t.Fatalf("broker max_deliver = %d", info.Config.MaxDeliver)
	}
}

// A hand-off that fails on the last attempt is retried on the next delivery instead of being lost, without running the handler again.
func TestJetStreamRetriesDeadLetterThatFailsOnLastAttempt(t *testing.T) {
	t.Parallel()
	client, cfg := startJetStream(t)
	send(t, producer(t, client), "facts.a", "evt-1")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	attempts, handoffs := 0, 0
	c, err := client.Consumer(deliveryConfig(t, jetstream.DeliveryPolicy{
		MaxAttempts: 2,
		OnDeadLetter: func(_ context.Context, m messaging.Message, cause error) error {
			handoffs++
			if handoffs == 1 {
				return errors.New("dead-letter store down")
			}
			if m.Headers[jetstream.MsgIDHeader] != "evt-1" || !errors.Is(cause, jetstream.ErrDeliveryLimit) {
				t.Errorf("handed off %+v: %v", m, cause)
			}
			cancel()
			return nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = c.Consume(ctx, func(context.Context, messaging.Message) error {
		attempts++
		return errors.New("database down")
	})
	if !errors.Is(err, context.Canceled) || attempts != 2 || handoffs != 2 {
		t.Fatalf("Consume = %v, attempts=%d handoffs=%d", err, attempts, handoffs)
	}
	waitSettled(t, rawConsumer(t, cfg))
}

// A process that dies during the hand-off neither acknowledges nor naks the message; the broker redelivers it after AckWait and the next process hands it off.
func TestJetStreamRedeliversAfterCrashDuringDeadLetter(t *testing.T) {
	t.Parallel()
	_, cfg := startJetStream(t)
	first, err := jetstream.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	send(t, producer(t, first), "facts.a", "evt-1")
	crashed := make(chan struct{})
	c, err := first.Consumer(deliveryConfig(t, jetstream.DeliveryPolicy{
		MaxAttempts: 1,
		OnDeadLetter: func(context.Context, messaging.Message, error) error {
			// The connection goes away mid hand-off, so no acknowledgement of any kind reaches the server.
			_ = first.Close()
			close(crashed)
			return errors.New("process died")
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	failing := func(context.Context, messaging.Message) error { return errors.New("database down") }
	go func() { _ = c.Consume(t.Context(), failing) }()
	<-crashed

	second := connect(t, cfg)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var mu sync.Mutex
	var handed []string
	again, err := second.Consumer(deliveryConfig(t, jetstream.DeliveryPolicy{
		MaxAttempts: 1,
		OnDeadLetter: func(_ context.Context, m messaging.Message, _ error) error {
			mu.Lock()
			defer mu.Unlock()
			handed = append(handed, m.Headers[jetstream.MsgIDHeader])
			cancel()
			return nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Consume(ctx, failing); !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v", err)
	}
	if len(handed) != 1 || handed[0] != "evt-1" {
		t.Fatalf("handed off %v", handed)
	}
	waitSettled(t, rawConsumer(t, cfg))
}

// An existing durable consumer with a server-side delivery limit is updated by a declaring consumer and refused by a binding one.
func TestJetStreamExistingConsumerWithDeliveryLimit(t *testing.T) {
	t.Parallel()
	client, cfg := startJetStream(t)
	js := rawConsumer(t, cfg)
	if _, err := js.CreateOrUpdateConsumer(t.Context(), "FACTS", natsjs.ConsumerConfig{
		Durable: "usage", FilterSubjects: []string{"facts.>"}, AckPolicy: natsjs.AckExplicitPolicy, MaxDeliver: 20,
	}); err != nil {
		t.Fatal(err)
	}

	bound := deliveryConfig(t, jetstream.DeliveryPolicy{})
	bound.Bind = true
	b, err := client.Consumer(bound)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Consume(t.Context(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, jetstream.ErrUnsafeBoundConsumer) {
		t.Fatalf("bind = %v", err)
	}

	d, err := client.Consumer(deliveryConfig(t, jetstream.DeliveryPolicy{}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_ = d.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
	if limit := consumerInfo(t, js).Config.MaxDeliver; limit != -1 {
		t.Fatalf("declared consumer kept max_deliver %d", limit)
	}
	if err := b.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); errors.Is(err, jetstream.ErrUnsafeBoundConsumer) {
		t.Fatal("bind still refused after the limit was removed")
	}
}

// A binding consumer refuses a durable consumer with ack-all, where acknowledging a later message settles an earlier failed one.
func TestJetStreamBindRefusesAckAll(t *testing.T) {
	t.Parallel()
	client, cfg := startJetStream(t)
	if _, err := rawConsumer(t, cfg).CreateOrUpdateConsumer(t.Context(), "FACTS", natsjs.ConsumerConfig{
		Durable: "usage", FilterSubjects: []string{"facts.>"}, AckPolicy: natsjs.AckAllPolicy, MaxDeliver: -1,
	}); err != nil {
		t.Fatal(err)
	}
	bound := deliveryConfig(t, jetstream.DeliveryPolicy{})
	bound.Bind = true
	b, err := client.Consumer(bound)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Consume(t.Context(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, jetstream.ErrUnsafeBoundConsumer) {
		t.Fatalf("bind = %v", err)
	}
}

// Registry consumers take their delivery policy from WithDeliveryPolicy.
func TestJetStreamRegistryDeliveryPolicy(t *testing.T) {
	t.Parallel()
	client, cfg := startJetStream(t)
	send(t, producer(t, client), "facts.a", "evt-1")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	handed := make(chan string, 1)
	reg := messaging.NewRegistry()
	if err := jetstream.Register(reg, cfg, jetstream.WithDeliveryPolicy(func(string) jetstream.DeliveryPolicy {
		return jetstream.DeliveryPolicy{MaxAttempts: 1, OnDeadLetter: func(_ context.Context, m messaging.Message, _ error) error {
			handed <- m.Headers[jetstream.MsgIDHeader]
			cancel()
			return nil
		}}
	})); err != nil {
		t.Fatal(err)
	}
	c, err := reg.NewConsumer(t.Context(), messaging.Config{
		Adapter: jetstream.AdapterName, DeliveryGuarantee: messaging.DeliveryAtLeastOnce,
		CommitStrategy: messaging.CommitAfterHandlerSuccess, ConsumerGroup: "audit",
	}, nil, "facts.a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return errors.New("down") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v", err)
	}
	if id := <-handed; id != "evt-1" {
		t.Fatalf("handed off %s", id)
	}
}
