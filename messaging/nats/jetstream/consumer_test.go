package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
)

func consumerConfig(report func(error)) ConsumerConfig {
	return ConsumerConfig{
		Stream: "FACTS", Durable: "usage", FilterSubjects: []string{"facts.*.>"}, AckWait: time.Minute, MaxAckPending: 20,
		Batch: 10, FetchWait: time.Second, RetryDelay: time.Millisecond, MaxRetryDelay: 4 * time.Millisecond, OnError: report,
	}
}

func TestConsumerAcksSuccessAndNaksFailureWithBackoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ok, failed, broken := &fakeMsg{seq: 1, delivered: 1}, &fakeMsg{seq: 2, delivered: 3}, &fakeMsg{seq: 3, metaErr: errors.New("no reply")}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, ok, failed, broken), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(context.DeadlineExceeded), nil },
	}}
	fake := &fakeJS{pull: pull, bindErrs: []error{errors.New("connection closed")}}
	var reported []error
	var seen []messaging.Message
	c, err := testClient(fake).Consumer(consumerConfig(func(err error) { reported = append(reported, err) }))
	if err != nil {
		t.Fatal(err)
	}
	err = c.Consume(ctx, func(_ context.Context, m messaging.Message) error {
		seen = append(seen, m)
		if m.Offset == 2 {
			return errors.New("database down")
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume returned %v", err)
	}
	if !ok.progressed || !failed.progressed {
		t.Fatal("messages were not marked in progress before handling")
	}
	if !ok.acked || failed.acked || failed.nakDelay != 4*time.Millisecond || broken.nakDelay != 4*time.Millisecond {
		t.Fatalf("ok acked=%v failed acked=%v nak=%v broken nak=%v", ok.acked, failed.acked, failed.nakDelay, broken.nakDelay)
	}
	m := seen[0]
	if len(seen) != 2 || m.Topic != "facts.a" || m.Headers[MsgIDHeader] != "id-1" || m.Key != "k" ||
		!m.Timestamp.Equal(time.Unix(1, 0)) || string(m.Payload) != "payload" {
		t.Fatalf("messages = %+v", seen)
	}
	// Handler failures stay with the handler; only the bind and metadata failures are adapter failures.
	if len(reported) != 2 {
		t.Fatalf("reported = %v", reported)
	}
	cc := fake.consumer
	if fake.boundTo != "FACTS" || cc.Durable != "usage" || cc.AckPolicy != natsjs.AckExplicitPolicy ||
		cc.DeliverPolicy != natsjs.DeliverAllPolicy || cc.MaxDeliver != -1 || cc.MaxAckPending != 20 || cc.FilterSubjects[0] != "facts.*.>" {
		t.Fatalf("bound %s with %+v", fake.boundTo, cc)
	}
}

func TestConsumerRebindsAfterDeletionAndResolvesStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pull := &fakeConsumer{}
	pull.fetches = []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return nil, natsjs.ErrConsumerDeleted },
		func() (natsjs.MessageBatch, error) { return batchOf(natsjs.ErrNoMessages), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}
	fake := &fakeJS{pull: pull}
	var reported int
	cfg := consumerConfig(func(error) { reported++ })
	cfg.Stream = ""
	c, err := testClient(fake).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = c.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
	if !errors.Is(err, context.Canceled) || reported != 1 || len(pull.sizes) != 3 {
		t.Fatalf("err=%v reported=%d fetches=%d", err, reported, len(pull.sizes))
	}
	if fake.bySubject != "facts.*.>" || fake.boundTo != "FOUND" {
		t.Fatalf("resolved %q to %q", fake.bySubject, fake.boundTo)
	}
}

func TestConsumerStopsWhenClosedAndRefusesNilHandler(t *testing.T) {
	t.Parallel()
	owned := testClient(&fakeJS{})
	c, err := newConsumer(owned.self, consumerConfig(func(error) {}), owned)
	if err != nil {
		t.Fatal(err)
	}
	if c.Topic() != "facts.*.>" {
		t.Fatalf("topic = %q", c.Topic())
	}
	if err := c.Consume(context.Background(), nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	if err := c.Close(); err != nil || c.Close() != nil {
		t.Fatal("close failed")
	}
	if err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("consume after close = %v", err)
	}
	if _, err := owned.api(); !errors.Is(err, messaging.ErrClosed) {
		t.Fatal("closing an owning consumer left its client open")
	}
	if _, err := testClient(&fakeJS{}).Consumer(ConsumerConfig{Durable: "x", OnError: func(error) {}}); err == nil {
		t.Fatal("consumer without stream or filter accepted")
	}
}

func TestOffsetClamps(t *testing.T) {
	t.Parallel()
	if offset(7) != 7 || offset(^uint64(0)) <= 0 {
		t.Fatal("offset mapping")
	}
}

func TestConsumerTerminatesTerminalFailures(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poison := &fakeMsg{seq: 1, delivered: 1}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, poison), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	c, err := testClient(&fakeJS{pull: pull}).Consumer(consumerConfig(func(error) {}))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Consume(ctx, func(ctx context.Context, _ messaging.Message) error {
		return fmt.Errorf("unreadable: %w", messaging.ErrTerminal)
	})
	if !poison.termed || poison.nakDelay != 0 || poison.acked {
		t.Fatalf("poison termed=%v nak=%v acked=%v", poison.termed, poison.nakDelay, poison.acked)
	}
}

func TestConsumerHandsExhaustedAndTerminalMessagesToDeadLetter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exhausted, poison, retried, lost := &fakeMsg{seq: 1, delivered: 3}, &fakeMsg{seq: 2, delivered: 1}, &fakeMsg{seq: 3, delivered: 2}, &fakeMsg{seq: 4, delivered: 1}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, exhausted, poison, retried, lost), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	var reported []error
	var dead []int64
	cfg := consumerConfig(func(err error) { reported = append(reported, err) })
	cfg.Delivery.MaxAttempts = 3
	dlqDown := errors.New("dlq down")
	cfg.Delivery.OnDeadLetter = func(_ context.Context, m messaging.Message, cause error) error {
		if m.Offset == 4 {
			return dlqDown
		}
		dead = append(dead, m.Offset)
		return nil
	}
	c, err := testClient(&fakeJS{pull: pull}).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Consume(ctx, func(_ context.Context, m messaging.Message) error {
		if m.Offset == 2 || m.Offset == 4 {
			return fmt.Errorf("unreadable: %w", messaging.ErrTerminal)
		}
		return errors.New("database down")
	})
	if !exhausted.termed || !poison.termed || retried.termed || retried.nakDelay == 0 || lost.termed || lost.nakDelay == 0 {
		t.Fatalf("termed exhausted=%v poison=%v retried=%v lost=%v", exhausted.termed, poison.termed, retried.termed, lost.termed)
	}
	if len(dead) != 2 || dead[0] != 1 || dead[1] != 2 {
		t.Fatalf("dead-lettered = %v", dead)
	}
	if len(reported) != 1 || !errors.Is(reported[0], dlqDown) {
		t.Fatalf("reported = %v", reported)
	}
}

func TestConsumerDiscardsExhaustedMessageOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	last := &fakeMsg{seq: 1, delivered: 2}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, last), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	var reported []error
	cfg := consumerConfig(func(err error) { reported = append(reported, err) })
	cfg.Delivery = DeliveryPolicy{MaxAttempts: 2, DiscardExhausted: true}
	c, err := testClient(&fakeJS{pull: pull}).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error { return errors.New("database down") })
	if !last.termed || len(reported) != 1 || !errors.Is(reported[0], ErrDeliveryLimit) {
		t.Fatalf("termed=%v reported=%v", last.termed, reported)
	}
}

func TestConsumerEndsOnPermanentBindFailure(t *testing.T) {
	t.Parallel()
	refused := &natsjs.APIError{Code: 400, ErrorCode: natsjs.ErrorCode(10012), Description: "deliver policy can not be updated"}
	missing := natsjs.ErrStreamNotFound
	var reported []error
	fake := &fakeJS{pull: &fakeConsumer{}, bindErrs: []error{missing, refused}}
	c, err := testClient(fake).Consumer(consumerConfig(func(err error) { reported = append(reported, err) }))
	if err != nil {
		t.Fatal(err)
	}
	err = c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil })
	var api *natsjs.APIError
	if !errors.As(err, &api) || api.Code != 400 {
		t.Fatalf("Consume = %v", err)
	}
	if len(reported) != 1 || !errors.Is(reported[0], natsjs.ErrStreamNotFound) {
		t.Fatalf("a missing stream must be retried, reported = %v", reported)
	}
}

func TestConsumerBindsWithoutDeclaring(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	fake := &fakeJS{pull: pull}
	cfg := consumerConfig(func(error) {})
	cfg.Bind = true
	c, err := testClient(fake).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if fake.looked != "usage" || fake.boundTo != "FACTS" || fake.consumer.Durable != "" {
		t.Fatalf("looked up %q on %q, declared %+v", fake.looked, fake.boundTo, fake.consumer)
	}
}

// A message delivered again after its last attempt failed and its hand-off failed retries only the hand-off.
func TestConsumerRetriesOnlyTheHandOffAfterTheLastAttempt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	again := &fakeMsg{seq: 1, delivered: 3}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, again), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	cfg := consumerConfig(func(error) {})
	cfg.Delivery.MaxAttempts = 2
	var causes []error
	cfg.Delivery.OnDeadLetter = func(_ context.Context, _ messaging.Message, cause error) error {
		causes = append(causes, cause)
		return nil
	}
	c, err := testClient(&fakeJS{pull: pull}).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handled := 0
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error { handled++; return nil })
	if handled != 0 || len(causes) != 1 || !errors.Is(causes[0], ErrDeliveryLimit) || !again.termed {
		t.Fatalf("handled=%d causes=%v termed=%v", handled, causes, again.termed)
	}
}

func TestConsumerRetriesUntilSuccessByDefault(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	late := &fakeMsg{seq: 1, delivered: 1000}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, late), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	var reported []error
	c, err := testClient(&fakeJS{pull: pull}).Consumer(consumerConfig(func(err error) { reported = append(reported, err) }))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error { return errors.New("database down") })
	if late.termed || late.nakDelay == 0 || len(reported) != 0 {
		t.Fatalf("termed=%v nak=%v reported=%v", late.termed, late.nakDelay, reported)
	}
}

func TestConsumerRefusesUnsafeBoundConsumer(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		server natsjs.ConsumerConfig
		want   string
	}{
		"delivery limit": {server: natsjs.ConsumerConfig{AckPolicy: natsjs.AckExplicitPolicy, MaxDeliver: 20}, want: "max_deliver 20"},
		"ack all":        {server: natsjs.ConsumerConfig{AckPolicy: natsjs.AckAllPolicy, MaxDeliver: -1}, want: "ack policy"},
		"ack none":       {server: natsjs.ConsumerConfig{AckPolicy: natsjs.AckNonePolicy, MaxDeliver: -1}, want: "ack policy"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pull := &fakeConsumer{info: natsjs.ConsumerInfo{Config: tc.server}}
			cfg := consumerConfig(func(error) {})
			cfg.Bind = true
			c, err := testClient(&fakeJS{pull: pull}).Consumer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			err = c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil })
			if !errors.Is(err, ErrUnsafeBoundConsumer) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Consume = %v", err)
			}
		})
	}
}

func TestConsumerCloseWakesBackoff(t *testing.T) {
	t.Parallel()
	failed := make(chan struct{}, 1)
	cfg := consumerConfig(func(error) {
		select {
		case failed <- struct{}{}:
		default:
		}
	})
	cfg.RetryDelay, cfg.MaxRetryDelay = time.Hour, time.Hour
	c, err := testClient(&fakeJS{pull: &fakeConsumer{}, bindErrs: []error{errTransient}}).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil })
	}()
	<-failed
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, messaging.ErrClosed) {
			t.Fatalf("Consume = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not wake the backoff")
	}
}

func TestConsumerReleasesUnhandledMessagesWhenStopped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second, third := &fakeMsg{seq: 1, delivered: 1}, &fakeMsg{seq: 2, delivered: 1}, &fakeMsg{seq: 3, delivered: 1}
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, first, second, third), nil },
	}}
	c, err := testClient(&fakeJS{pull: pull}).Consumer(consumerConfig(func(error) {}))
	if err != nil {
		t.Fatal(err)
	}
	handled := 0
	err = c.Consume(ctx, func(ctx context.Context, m messaging.Message) error {
		handled++
		if m.Offset == 2 {
			cancel()
			return ctx.Err()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || handled != 2 {
		t.Fatalf("err=%v handled=%d", err, handled)
	}
	if !first.acked || !second.naked || second.nakDelay != 0 || !third.naked || third.progressed {
		t.Fatalf("first acked=%v second naked=%v third naked=%v progressed=%v", first.acked, second.naked, third.naked, third.progressed)
	}
}

func TestConsumerRebindsOverANewClient(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second := &fakeJS{pull: &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}}
	current := testClient(nil)
	first := &fakeJS{pull: &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { current = testClient(second); return batchOf(nil), nil },
	}}}
	current = testClient(first)
	c, err := newConsumer(func() (*Client, error) { return current, nil }, consumerConfig(func(error) {}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if first.boundTo != "FACTS" || second.boundTo != "FACTS" {
		t.Fatalf("bound first=%q second=%q", first.boundTo, second.boundTo)
	}
}

// A server restart can lose the durable consumer behind the handle, so any failed fetch declares it again.
func TestConsumerRebindsAfterAFailedFetch(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return nil, natsgo.ErrNoResponders },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	fake := &fakeJS{pull: pull}
	c, err := testClient(fake).Consumer(consumerConfig(func(error) {}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if fake.binds != 2 {
		t.Fatalf("binds = %d", fake.binds)
	}
}

// A request lost during a reconnect or refused by a permission gets no reply, so every API request has a bound.
func TestJetStreamRequestsAreBounded(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	pull := &fakeConsumer{info: natsjs.ConsumerInfo{Name: "r"}, fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	fake := &fakeJS{pull: pull}
	client := testClient(fake)
	if err := client.EnsureStream(context.Background(), StreamConfig{Name: "FACTS", Subjects: []string{"facts.>"}, MaxAge: time.Hour, MaxBytes: 1 << 20, MaxMsgSize: 1 << 10, Duplicates: time.Minute}); err != nil {
		t.Fatal(err)
	}
	for _, bind := range []bool{false, true} {
		cfg := consumerConfig(func(error) {})
		cfg.Stream, cfg.Bind = "", bind
		c, err := client.Consumer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
		ctx, cancel = context.WithCancel(context.Background())
	}
	cancel()
	if _, err := client.Replay(context.Background(), ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartTime: time.Unix(1, 0), Limit: 1, Wait: time.Second}); err != nil {
		t.Fatal(err)
	}
	if len(fake.unbounded) != 0 {
		t.Fatalf("requests without a deadline: %v", fake.unbounded)
	}
}

// A consumer that owns its stream declares it, creating or updating it, each time it binds: first, and again after a failed fetch, since the stream may be gone after a server restart.
func TestConsumerDeclaresItsStreamOnEveryBind(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return nil, errors.New("consumer gone") },
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	fake := &fakeJS{pull: pull}
	var reported []error
	cfg := consumerConfig(func(err error) { reported = append(reported, err) })
	stream := StreamConfig{Name: "FACTS", Subjects: []string{"facts.>"}, MaxAge: time.Hour, MaxBytes: 1 << 20, MaxMsgSize: 1 << 10, Duplicates: time.Minute}
	cfg.Declare = &stream
	c, err := testClient(fake).Consumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Consume(ctx, func(context.Context, messaging.Message) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if fake.stream.Name != "FACTS" || fake.stream.MaxAge != time.Hour || fake.declares != 2 || fake.binds != 2 || len(reported) != 1 {
		t.Fatalf("stream %+v, declares %d, binds %d, reported %v", fake.stream, fake.declares, fake.binds, reported)
	}
}

// Without Declare the consumer never declares the stream.
func TestConsumerWithoutDeclareLeavesTheStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { cancel(); return batchOf(nil), nil },
	}}
	fake := &fakeJS{pull: pull}
	c, err := testClient(fake).Consumer(consumerConfig(func(error) {}))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
	if fake.declares != 0 {
		t.Fatalf("declares %d", fake.declares)
	}
}

// ConsumeStatus reports a failure until a fetch succeeds, and the last failure while the loop retries.
func TestConsumerStatusFollowsFetches(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c *Consumer
	var seen []error
	gone := errors.New("consumer gone")
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { seen = append(seen, c.ConsumeStatus()); return batchOf(nil), nil },
		func() (natsjs.MessageBatch, error) { seen = append(seen, c.ConsumeStatus()); return nil, gone },
		func() (natsjs.MessageBatch, error) {
			seen = append(seen, c.ConsumeStatus())
			cancel()
			return batchOf(nil), nil
		},
	}}
	var err error
	if c, err = testClient(&fakeJS{pull: pull}).Consumer(consumerConfig(func(error) {})); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(c.ConsumeStatus(), ErrNotReceiving) {
		t.Fatalf("status before Consume = %v", c.ConsumeStatus())
	}
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
	if len(seen) != 3 || !errors.Is(seen[0], ErrNotReceiving) || seen[1] != nil || !errors.Is(seen[2], gone) {
		t.Fatalf("statuses %v", seen)
	}
}

// A fetch the server never completes, such as a pull request a missing permission drops, ends at the fetch deadline rather than with the server's idle reply. It is a failure that keeps the binding, not an empty fetch.
func TestConsumerTreatsAnIncompleteFetchAsAFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c *Consumer
	var seen, reported []error
	pull := &fakeConsumer{fetches: []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(context.DeadlineExceeded, &fakeMsg{seq: 1}), nil },
		func() (natsjs.MessageBatch, error) {
			seen = append(seen, c.ConsumeStatus())
			return batchOf(nil), nil
		},
		func() (natsjs.MessageBatch, error) {
			seen = append(seen, c.ConsumeStatus())
			cancel()
			return batchOf(nil), nil
		},
	}}
	fake := &fakeJS{pull: pull}
	var err error
	if c, err = testClient(fake).Consumer(consumerConfig(func(err error) { reported = append(reported, err) })); err != nil {
		t.Fatal(err)
	}
	handled := 0
	var handling error
	_ = c.Consume(ctx, func(context.Context, messaging.Message) error {
		handled++
		handling = c.ConsumeStatus()
		return nil
	})
	if handled != 1 || handling != nil {
		t.Fatalf("handled %d messages of the incomplete batch, status while handling %v", handled, handling)
	}
	if len(seen) != 2 || !errors.Is(seen[0], ErrFetchIncomplete) || seen[1] != nil {
		t.Fatalf("statuses %v", seen)
	}
	if len(reported) != 1 || !errors.Is(reported[0], ErrFetchIncomplete) || !(ErrorClassifier{}).IsRetryableError(reported[0]) {
		t.Fatalf("reported %v", reported)
	}
	if fake.binds != 1 {
		t.Fatalf("binds = %d", fake.binds)
	}
}

func TestConsumerDeclareMustMatchItsStream(t *testing.T) {
	t.Parallel()
	stream := StreamConfig{Name: "FACTS", Subjects: []string{"facts.>"}, MaxAge: time.Hour, MaxBytes: 1 << 20, MaxMsgSize: 1 << 10, Duplicates: time.Minute}
	for name, mutate := range map[string]func(*ConsumerConfig){
		"other stream": func(c *ConsumerConfig) { other := stream; other.Name = "OTHER"; c.Declare = &other },
		"no stream":    func(c *ConsumerConfig) { c.Stream, c.Declare = "", &stream },
		"bind":         func(c *ConsumerConfig) { c.Bind, c.Declare = true, &stream },
		"invalid":      func(c *ConsumerConfig) { bad := stream; bad.MaxAge = 0; c.Declare = &bad },
	} {
		cfg := consumerConfig(func(error) {})
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: declare accepted", name)
		}
	}
}
