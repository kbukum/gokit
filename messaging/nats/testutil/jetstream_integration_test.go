//go:build integration

package testutil_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/messaging"
	gokitnats "github.com/kbukum/gokit/messaging/nats"
	"github.com/kbukum/gokit/messaging/nats/jetstream"
	"github.com/kbukum/gokit/messaging/nats/testutil"
)

var stream = jetstream.StreamConfig{
	Name: "FACTS", Subjects: []string{"facts.>"}, MaxAge: time.Hour, MaxBytes: 16 << 20, MaxMsgSize: 16 << 10,
	Duplicates: 2 * time.Minute,
}

func startFixture(t *testing.T, opts ...testutil.Option) *testutil.JetStreamFixture {
	t.Helper()
	f, err := testutil.StartJetStream(t.Context(), t.TempDir(), opts...)
	if f != nil {
		t.Cleanup(func() {
			if err := f.Close(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func connect(t *testing.T, cfg gokitnats.Config) *jetstream.Client {
	t.Helper()
	client, err := jetstream.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}

func startJetStream(t *testing.T) (*jetstream.Client, gokitnats.Config) {
	t.Helper()
	cfg := startFixture(t).Config()
	client := connect(t, cfg)
	if err := client.EnsureStream(t.Context(), stream); err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureStream(t.Context(), stream); err != nil {
		t.Fatalf("ensure stream twice: %v", err)
	}
	return client, cfg
}

func producer(t *testing.T, client *jetstream.Client) *jetstream.Producer {
	t.Helper()
	p, err := client.Producer(jetstream.ProducerConfig{Stream: "FACTS"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func send(t *testing.T, p messaging.Producer, subject, id string) {
	t.Helper()
	if err := p.Send(t.Context(), messaging.NewMessage(subject, "", []byte(id), map[string]string{jetstream.MsgIDHeader: id})); err != nil {
		t.Fatal(err)
	}
}

func replayAll(t *testing.T, client *jetstream.Client) []messaging.Message {
	t.Helper()
	page, err := client.Replay(t.Context(), jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: 100, Wait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return page.Messages
}

func TestJetStreamPublishDedupAndRefusals(t *testing.T) {
	t.Parallel()
	client, _ := startJetStream(t)
	p := producer(t, client)
	send(t, p, "facts.acme.web.run", "evt-1")
	send(t, p, "facts.acme.web.run", "evt-1")
	send(t, p, "facts.acme.web.run", "evt-2")
	if got := replayAll(t, client); len(got) != 2 || got[0].Offset != 1 || got[1].Headers[jetstream.MsgIDHeader] != "evt-2" {
		t.Fatalf("stored = %+v", got)
	}
	if err := p.PublishBinary(t.Context(), "facts.a", "", make([]byte, 17<<10)); err == nil {
		t.Fatal("oversized message was stored")
	}
	if err := p.PublishBinary(t.Context(), "other.a", "", nil); err == nil {
		t.Fatal("message outside every stream was acknowledged")
	}
}

func TestJetStreamDurableConsumerRedeliversAfterNak(t *testing.T) {
	t.Parallel()
	client, _ := startJetStream(t)
	p := producer(t, client)
	send(t, p, "facts.acme.web.a", "evt-1")
	send(t, p, "facts.acme.web.b", "evt-2")

	cfg := jetstream.ConsumerConfig{
		Durable: "usage", FilterSubjects: []string{"facts.>"}, AckWait: 5 * time.Second, MaxAckPending: 10, Batch: 5,
		FetchWait: 200 * time.Millisecond, RetryDelay: 50 * time.Millisecond, MaxRetryDelay: 200 * time.Millisecond,
		OnError: func(err error) { t.Log(err) },
	}
	c, err := client.Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var mu sync.Mutex
	var seen []string
	attempts := map[string]int{}
	err = c.Consume(ctx, func(_ context.Context, m messaging.Message) error {
		mu.Lock()
		defer mu.Unlock()
		id := m.Headers[jetstream.MsgIDHeader]
		attempts[id]++
		if id == "evt-1" && attempts[id] == 1 {
			return errors.New("transient")
		}
		seen = append(seen, id)
		if len(seen) == 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume returned %v", err)
	}
	if attempts["evt-1"] != 2 || attempts["evt-2"] != 1 || seen[0] != "evt-2" || seen[1] != "evt-1" {
		t.Fatalf("attempts=%v seen=%v", attempts, seen)
	}

	// Acknowledged messages are not delivered again to the same durable consumer, even through a new consumer value.
	again, err := client.Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_ = again.Consume(ctx, func(_ context.Context, m messaging.Message) error {
		t.Errorf("redelivered acknowledged %s", m.Headers[jetstream.MsgIDHeader])
		return nil
	})
}

func TestJetStreamRegistryRoundTrip(t *testing.T) {
	t.Parallel()
	_, cfg := startJetStream(t)
	reg := messaging.NewRegistry()
	if err := jetstream.Register(reg, cfg); err != nil {
		t.Fatal(err)
	}
	common := messaging.Config{
		Adapter: jetstream.AdapterName, DeliveryGuarantee: messaging.DeliveryAtLeastOnce,
		CommitStrategy: messaging.CommitAfterHandlerSuccess, ConsumerGroup: "audit",
	}
	p, err := reg.NewProducer(t.Context(), common, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	event := messaging.Event{ID: "evt-7", Type: "run.ended", Source: "runs", Subject: "run-7"}
	for range 2 {
		if err := p.Publish(t.Context(), "facts.acme.web.ended", event); err != nil {
			t.Fatal(err)
		}
	}
	c, err := reg.NewConsumer(t.Context(), common, nil, "facts.acme.web.ended")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var got []messaging.Message
	err = c.Consume(ctx, func(_ context.Context, m messaging.Message) error {
		got = append(got, m)
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || len(got) != 1 || got[0].Key != "run-7" || got[0].Headers["event-type"] != "run.ended" {
		t.Fatalf("err=%v got=%+v", err, got)
	}
}

func TestJetStreamReplayFromSequenceAndTime(t *testing.T) {
	t.Parallel()
	client, _ := startJetStream(t)
	p := producer(t, client)
	send(t, p, "facts.acme.web.a", "a-1")
	send(t, p, "facts.globex.app.a", "g-1")
	send(t, p, "facts.acme.web.b", "a-2")
	time.Sleep(20 * time.Millisecond)
	cut := time.Now()
	time.Sleep(20 * time.Millisecond)
	send(t, p, "facts.acme.api.a", "a-3")

	id := func(m messaging.Message) string { return m.Headers[jetstream.MsgIDHeader] }
	req := jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.acme.>", StartSequence: 1, Limit: 2, Wait: 5 * time.Second}
	page, err := client.Replay(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || id(page.Messages[0]) != "a-1" || id(page.Messages[1]) != "a-2" || !page.More || page.Next != 4 {
		t.Fatalf("page 1 = %+v", page)
	}
	req.StartSequence = page.Next
	page, err = client.Replay(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || id(page.Messages[0]) != "a-3" || page.More || page.Next != 5 {
		t.Fatalf("page 2 = %+v", page)
	}
	req.StartSequence = page.Next
	if page, err = client.Replay(t.Context(), req); err != nil || len(page.Messages) != 0 || page.Next != 5 {
		t.Fatalf("page 3 = %+v err=%v", page, err)
	}

	page, err = client.Replay(t.Context(), jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.acme.>", StartTime: cut, Limit: 10, Wait: 5 * time.Second})
	if err != nil || len(page.Messages) != 1 || id(page.Messages[0]) != "a-3" {
		t.Fatalf("time page = %+v err=%v", page, err)
	}
	page, err = client.Replay(t.Context(), jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.acme.>", StartTime: time.Now().Add(time.Hour), Limit: 10, Wait: time.Second})
	if err != nil || len(page.Messages) != 0 || page.Next != 5 {
		t.Fatalf("future page = %+v err=%v", page, err)
	}
}

func TestJetStreamRefusesWrongPassword(t *testing.T) {
	t.Parallel()
	_, cfg := startJetStream(t)
	wrong := filepath.Join(t.TempDir(), "wrong")
	if err := os.WriteFile(wrong, []byte("not-the-password"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.PasswordFile = wrong
	cfg.PublishTimeout = "1s"
	client, err := jetstream.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := producer(t, client).PublishBinary(t.Context(), "facts.a", "", nil); err == nil {
		t.Fatal("publication with a wrong password was acknowledged")
	}
}

func TestJetStreamComponentHealthyWhenConnected(t *testing.T) {
	_, cfg := startJetStream(t)
	c, err := jetstream.NewComponent("broker", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	deadline := time.Now().Add(10 * time.Second)
	for c.Health(t.Context()).Status != component.StatusHealthy {
		if time.Now().After(deadline) {
			t.Fatalf("health = %+v", c.Health(t.Context()))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A client and a running consumer survive a broker restart: the client reconnects without a limit by default and the consumer declares its durable consumer again once the stream exists.
func TestJetStreamSurvivesBrokerRestart(t *testing.T) {
	t.Parallel()
	f := startFixture(t)
	cfg := f.Config()
	cfg.ReconnectWait = "100ms"
	client := connect(t, cfg)
	if err := client.EnsureStream(t.Context(), stream); err != nil {
		t.Fatal(err)
	}
	declared := stream
	c, err := client.Consumer(jetstream.ConsumerConfig{
		Stream: stream.Name, Declare: &declared, Durable: "usage", FilterSubjects: []string{"facts.>"}, FetchWait: 200 * time.Millisecond,
		RetryDelay: 50 * time.Millisecond, MaxRetryDelay: 200 * time.Millisecond, OnError: func(err error) { t.Log(err) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	got := make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(ctx, func(_ context.Context, m messaging.Message) error {
			got <- m.Headers[jetstream.MsgIDHeader]
			return nil
		})
	}()
	p := producer(t, client)
	send(t, p, "facts.a", "before")
	if id := <-got; id != "before" {
		t.Fatalf("got %s", id)
	}

	if err := f.Restart(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Stream data lived in memory, so the stream is gone until the consumer, which owns it, declares it again.
	deadline := time.Now().Add(30 * time.Second)
	for p.Send(t.Context(), messaging.NewMessage("facts.a", "", []byte("after"), map[string]string{jetstream.MsgIDHeader: "after"})) != nil {
		if time.Now().After(deadline) {
			t.Fatal("the consumer did not declare its stream again after the broker restarted")
		}
		time.Sleep(100 * time.Millisecond)
	}
	select {
	case id := <-got:
		if id != "after" {
			t.Fatalf("got %s", id)
		}
	case err := <-done:
		t.Fatalf("Consume ended: %v", err)
	case <-ctx.Done():
		t.Fatal("consumer did not resume after the broker restarted")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v", err)
	}
}

// Each operation works with exactly the rights the package documentation lists. The server refuses a request without the right by not answering it, so a consumer lacking the create right reports bounded timeouts and handles nothing. Each user may only receive replies under its own inbox prefix, so users cannot read each other's replies.
func TestJetStreamDocumentedPermissions(t *testing.T) {
	t.Parallel()
	inbox := func(name string) string { return `subscribe: ["_INBOX.` + name + `.>"]` }
	f := startFixture(t,
		testutil.WithUser("producer", `publish: ["facts.>"], `+inbox("producer")),
		testutil.WithUser("admin", `publish: ["$JS.API.STREAM.INFO.FACTS", "$JS.API.STREAM.CREATE.FACTS", "$JS.API.STREAM.UPDATE.FACTS"], `+inbox("admin")),
		testutil.WithUser("declarer", `publish: ["$JS.API.CONSUMER.CREATE.FACTS.>", "$JS.API.CONSUMER.MSG.NEXT.FACTS.usage", "$JS.ACK.FACTS.usage.>"], `+inbox("declarer")),
		testutil.WithUser("binder", `publish: ["$JS.API.CONSUMER.INFO.FACTS.usage", "$JS.API.CONSUMER.MSG.NEXT.FACTS.usage", "$JS.ACK.FACTS.usage.>"], `+inbox("binder")),
		testutil.WithUser("puller", `publish: ["$JS.API.CONSUMER.CREATE.FACTS.>"], `+inbox("puller")),
		testutil.WithUser("replayer", `publish: ["$JS.API.STREAM.INFO.FACTS", "$JS.API.CONSUMER.CREATE.FACTS.>", "$JS.API.CONSUMER.MSG.NEXT.FACTS.*", "$JS.API.CONSUMER.DELETE.FACTS.*"], `+inbox("replayer")),
	)
	config := func(name string) gokitnats.Config {
		cfg, err := f.ConfigFor(name)
		if err != nil {
			t.Fatal(err)
		}
		cfg.PublishTimeout = "2s"
		cfg.InboxPrefix = "_INBOX." + name
		return cfg
	}
	as := func(name string) *jetstream.Client { return connect(t, config(name)) }
	if err := as("admin").EnsureStream(t.Context(), stream); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}
	p, err := as("producer").Producer(jetstream.ProducerConfig{Stream: "FACTS"})
	if err != nil {
		t.Fatal(err)
	}
	send(t, p, "facts.a", "evt-1")
	send(t, p, "facts.b", "evt-2")

	// Without its prefix the producer's acks go to the shared inbox, which it may not read: the broker stores the fact but the producer never learns it.
	shared := config("producer")
	shared.InboxPrefix = ""
	unprefixed, err := connect(t, shared).Producer(jetstream.ProducerConfig{Stream: "FACTS"})
	if err != nil {
		t.Fatal(err)
	}
	if err := unprefixed.Send(t.Context(), messaging.NewMessage("facts.a", "", []byte("evt-3"), map[string]string{jetstream.MsgIDHeader: "evt-3"})); err == nil {
		t.Fatal("publish without the inbox prefix was acknowledged")
	}

	consume := func(client *jetstream.Client, bind bool, wait time.Duration) (handled, reported int, err error) {
		var mu sync.Mutex
		c, err := client.Consumer(jetstream.ConsumerConfig{
			Stream: "FACTS", Durable: "usage", FilterSubjects: []string{"facts.>"}, Bind: bind,
			FetchWait: 200 * time.Millisecond, RetryDelay: 50 * time.Millisecond, MaxRetryDelay: 200 * time.Millisecond,
			OnError: func(err error) {
				mu.Lock()
				defer mu.Unlock()
				reported++
				t.Log(err)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), wait)
		defer cancel()
		err = c.Consume(ctx, func(context.Context, messaging.Message) error {
			handled++
			cancel()
			return nil
		})
		mu.Lock()
		defer mu.Unlock()
		return handled, reported, err
	}
	if handled, reported, err := consume(as("binder"), false, 5*time.Second); handled != 0 || reported == 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("without the create right: handled=%d reported=%d err=%v", handled, reported, err)
	}
	if handled, _, err := consume(as("declarer"), false, 20*time.Second); handled != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("declare: handled=%d err=%v", handled, err)
	}
	if handled, _, err := consume(as("binder"), true, 20*time.Second); handled != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("bind: handled=%d err=%v", handled, err)
	}

	// Without the pull right the server drops each fetch without a reply: the consumer reports it and stays degraded rather than looking idle.
	puller, err := as("puller").Consumer(jetstream.ConsumerConfig{
		Stream: "FACTS", Durable: "usage", FilterSubjects: []string{"facts.>"},
		FetchWait: time.Second, RetryDelay: 50 * time.Millisecond, MaxRetryDelay: 200 * time.Millisecond,
		OnError: func(err error) { t.Log(err) },
	})
	if err != nil {
		t.Fatal(err)
	}
	pullCtx, stopPull := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopPull()
	pulled := 0
	go func() {
		for pullCtx.Err() == nil && !errors.Is(puller.ConsumeStatus(), jetstream.ErrFetchIncomplete) {
			time.Sleep(20 * time.Millisecond)
		}
		stopPull()
	}()
	if err := puller.Consume(pullCtx, func(context.Context, messaging.Message) error { pulled++; return nil }); pulled != 0 ||
		!errors.Is(puller.ConsumeStatus(), jetstream.ErrFetchIncomplete) || !errors.Is(err, context.Canceled) {
		t.Fatalf("without the pull right: handled=%d status=%v err=%v", pulled, puller.ConsumeStatus(), err)
	}

	replayer := as("replayer")
	page, err := replayer.Replay(t.Context(), jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: 10, Wait: 5 * time.Second})
	if err != nil || len(page.Messages) != 3 {
		t.Fatalf("replay by sequence: %+v %v", page, err)
	}
	page, err = replayer.Replay(t.Context(), jetstream.ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartTime: time.Unix(1, 0), Limit: 10, Wait: 5 * time.Second})
	if err != nil || len(page.Messages) != 3 {
		t.Fatalf("replay by time: %+v %v", page, err)
	}
}
