package consumer

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

type fakeKafkaReader struct {
	fetch     chan kafkago.Message
	readErr   error
	commitErr error
	committed int
	closed    bool
}

func (r *fakeKafkaReader) next(ctx context.Context) (kafkago.Message, error) {
	if r.readErr != nil {
		return kafkago.Message{}, r.readErr
	}
	select {
	case <-ctx.Done():
		return kafkago.Message{}, ctx.Err()
	case msg, ok := <-r.fetch:
		if !ok {
			<-ctx.Done()
			return kafkago.Message{}, ctx.Err()
		}
		return msg, nil
	}
}

func (r *fakeKafkaReader) ReadMessage(ctx context.Context) (kafkago.Message, error) {
	return r.next(ctx)
}

func (r *fakeKafkaReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	return r.next(ctx)
}

func (r *fakeKafkaReader) CommitMessages(_ context.Context, msgs ...kafkago.Message) error {
	if r.commitErr != nil {
		return r.commitErr
	}
	r.committed += len(msgs)
	return nil
}

func (r *fakeKafkaReader) Stats() kafkago.ReaderStats { return kafkago.ReaderStats{} }

func (r *fakeKafkaReader) Close() error { r.closed = true; return nil }

func newTestConsumer(r kafkaReader, strategy messaging.CommitStrategy) *Consumer {
	var errCount atomic.Int64
	return &Consumer{
		reader:         r,
		topic:          "orders",
		log:            logging.New(&logging.Config{Level: "error"}, "test").WithComponent("kafka.consumer"),
		errCount:       &errCount,
		commitStrategy: strategy,
	}
}

func TestConsumerDeliversMessageAndLogsRecovery(t *testing.T) {
	fetch := make(chan kafkago.Message, 1)
	fetch <- kafkago.Message{Topic: "orders", Value: []byte("payload"), Offset: 3}
	r := &fakeKafkaReader{fetch: fetch}
	c := newTestConsumer(r, messaging.CommitAuto)
	c.errCount.Store(2) // simulate a prior connection blip to exercise the recovery branch

	ctx, cancel := context.WithCancel(context.Background())
	var got messaging.Message
	err := c.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
		got = msg
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v, want context.Canceled", err)
	}
	if string(got.Value) != "payload" {
		t.Fatalf("delivered value = %q", got.Value)
	}
	if c.errCount.Load() != 0 {
		t.Fatalf("errCount = %d, want reset to 0 after recovery", c.errCount.Load())
	}
}

func TestConsumerCommitsAfterHandlerSuccess(t *testing.T) {
	fetch := make(chan kafkago.Message, 1)
	fetch <- kafkago.Message{Topic: "orders", Value: []byte("x"), Offset: 7}
	r := &fakeKafkaReader{fetch: fetch}
	c := newTestConsumer(r, messaging.CommitAfterHandlerSuccess)

	ctx, cancel := context.WithCancel(context.Background())
	err := c.Consume(ctx, func(_ context.Context, _ messaging.Message) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume = %v, want context.Canceled", err)
	}
	if r.committed != 1 {
		t.Fatalf("committed = %d, want commit after successful handler", r.committed)
	}
}

func TestConsumerSurfacesCommitFailure(t *testing.T) {
	fetch := make(chan kafkago.Message, 1)
	fetch <- kafkago.Message{Topic: "orders", Value: []byte("x")}
	commitErr := errors.New("commit rejected during rebalance")
	r := &fakeKafkaReader{fetch: fetch, commitErr: commitErr}
	c := newTestConsumer(r, messaging.CommitAfterHandlerSuccess)

	err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return nil })
	if !errors.Is(err, commitErr) {
		t.Fatalf("Consume = %v, want commit failure surfaced (not silently dropped)", err)
	}
}

func TestConsumerReturnsHandlerErrorWithoutCommitting(t *testing.T) {
	fetch := make(chan kafkago.Message, 1)
	fetch <- kafkago.Message{Topic: "orders", Value: []byte("x")}
	r := &fakeKafkaReader{fetch: fetch}
	c := newTestConsumer(r, messaging.CommitAfterHandlerSuccess)
	handlerErr := errors.New("handler failed")

	err := c.Consume(context.Background(), func(context.Context, messaging.Message) error { return handlerErr })
	if !errors.Is(err, handlerErr) {
		t.Fatalf("Consume = %v, want handler error", err)
	}
	if r.committed != 0 {
		t.Fatalf("committed = %d, want no commit on handler failure (offset stays for redelivery)", r.committed)
	}
}

func TestConsumerReadErrorBacksOffThenHonorsCancellation(t *testing.T) {
	r := &fakeKafkaReader{readErr: errors.New("transient read error")}
	c := newTestConsumer(r, messaging.CommitAuto)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := c.Consume(ctx, func(context.Context, messaging.Message) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Consume = %v, want DeadlineExceeded during backoff", err)
	}
	if c.failures == 0 {
		t.Fatal("expected read error to increment the failure counter (backoff path)")
	}
}

func TestConsumerCloseClosesReader(t *testing.T) {
	r := &fakeKafkaReader{}
	c := newTestConsumer(r, messaging.CommitAuto)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !r.closed {
		t.Fatal("Close must close the underlying reader")
	}
}

func TestConsumerStatsDelegatesToReader(t *testing.T) {
	c := newTestConsumer(&fakeKafkaReader{}, messaging.CommitAuto)
	if got := c.Stats(); got.Topic != "" {
		t.Fatalf("Stats = %#v, want zero-value from fake reader", got)
	}
}

func TestAsRunnerWrapsConsumer(t *testing.T) {
	c := newTestConsumer(&fakeKafkaReader{}, messaging.CommitAuto)
	runner := AsRunner(c, func(context.Context, messaging.Message) error { return nil })
	if runner == nil {
		t.Fatal("AsRunner returned nil runner")
	}
	if runner.Topic() != "orders" {
		t.Fatalf("runner topic = %q, want orders", runner.Topic())
	}
}

func TestNewManagedConsumerExposesGroupID(t *testing.T) {
	mc, err := NewManagedConsumer(ManagedConsumerConfig{
		Common:  messaging.Config{Adapter: "kafka", ConsumerGroup: "workers"},
		Config:  kafka.Config{Brokers: []string{"127.0.0.1:1"}},
		Topic:   "events",
		Handler: func(context.Context, messaging.Message) error { return nil },
		Log:     logging.New(&logging.Config{Level: "error"}, "test"),
	})
	if err != nil {
		t.Fatalf("NewManagedConsumer: %v", err)
	}
	if mc.GroupID() != "workers" {
		t.Fatalf("GroupID = %q, want workers", mc.GroupID())
	}
}
