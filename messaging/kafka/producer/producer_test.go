package producer

import (
	"context"
	"errors"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

type fakeKafkaWriter struct {
	messages []kafkago.Message
	writes   int
	writeErr error
	closed   bool
	closeErr error
}

func (w *fakeKafkaWriter) WriteMessages(_ context.Context, msgs ...kafkago.Message) error {
	w.writes++
	if w.writeErr != nil {
		return w.writeErr
	}
	w.messages = append(w.messages, msgs...)
	return nil
}

func (w *fakeKafkaWriter) Stats() kafkago.WriterStats { return kafkago.WriterStats{} }

func (w *fakeKafkaWriter) Close() error { w.closed = true; return w.closeErr }

func newTestProducer(w kafkaWriter, attempts int, backoff time.Duration) *Producer {
	return &Producer{
		writer:        w,
		retryAttempts: attempts,
		retryBackoff:  backoff,
		log:           logging.New(&logging.Config{Level: "error"}, "test").WithComponent("kafka.producer"),
	}
}

func TestProducerWriteMessagesDeliversToWriter(t *testing.T) {
	w := &fakeKafkaWriter{}
	p := newTestProducer(w, 1, 0)

	err := p.WriteMessages(context.Background(), kafkago.Message{Topic: "orders", Value: []byte("payload")})
	if err != nil {
		t.Fatalf("WriteMessages: %v", err)
	}
	if w.writes != 1 || len(w.messages) != 1 {
		t.Fatalf("writes=%d messages=%d, want single write", w.writes, len(w.messages))
	}
	if got := string(w.messages[0].Value); got != "payload" {
		t.Fatalf("delivered value = %q", got)
	}
}

func TestProducerWriteMessagesPropagatesWriteError(t *testing.T) {
	writeErr := errors.New("broker rejected write")
	w := &fakeKafkaWriter{writeErr: writeErr}
	p := newTestProducer(w, 1, 0)

	err := p.WriteMessages(context.Background(), kafkago.Message{Topic: "orders", Value: []byte("x")})
	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteMessages error = %v, want write error surfaced (not swallowed)", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1 (no retry configured)", w.writes)
	}
}

func TestProducerWriteMessagesRetriesThenExhausts(t *testing.T) {
	writeErr := errors.New("temporary blip")
	w := &fakeKafkaWriter{writeErr: writeErr}
	p := newTestProducer(w, 3, time.Millisecond)

	err := p.WriteMessages(context.Background(), kafkago.Message{Topic: "orders", Value: []byte("x")})
	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteMessages error = %v, want exhausted write error", err)
	}
	if w.writes != 3 {
		t.Fatalf("writes = %d, want 3 bounded attempts", w.writes)
	}
}

func TestProducerWriteMessagesStopsRetryingOnCanceledContext(t *testing.T) {
	writeErr := errors.New("blip")
	w := &fakeKafkaWriter{writeErr: writeErr}
	p := newTestProducer(w, 5, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.WriteMessages(ctx, kafkago.Message{Topic: "orders", Value: []byte("x")})
	if err == nil {
		t.Fatal("WriteMessages with canceled context: want error")
	}
	if w.writes > 1 {
		t.Fatalf("writes = %d, want no retries after cancellation", w.writes)
	}
}

func TestProducerCloseClosesWriter(t *testing.T) {
	w := &fakeKafkaWriter{}
	p := newTestProducer(w, 1, 0)

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !w.closed {
		t.Fatal("Close must close the underlying writer")
	}
}

func TestProducerConvenienceMethodsRouteThroughWriter(t *testing.T) {
	w := &fakeKafkaWriter{}
	p := newTestProducer(w, 1, 0)
	ctx := context.Background()

	if err := p.Send(ctx, messaging.Message{Topic: "orders", Key: "k", Value: []byte("send")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := p.SendBatch(ctx, []messaging.Message{{Topic: "orders", Value: []byte("a")}, {Topic: "orders", Value: []byte("b")}}); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if err := p.PublishJSON(ctx, "orders", "jk", map[string]string{"id": "1"}); err != nil {
		t.Fatalf("PublishJSON: %v", err)
	}
	if err := p.PublishBinary(ctx, "orders", "bk", []byte("raw")); err != nil {
		t.Fatalf("PublishBinary: %v", err)
	}
	event := messaging.Event{ID: "evt-1", Type: "created", Source: "test", Subject: "subject-key", Timestamp: time.Unix(1, 0)}
	if err := p.Publish(ctx, "orders", event); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Send(1) + SendBatch(1 batched write) + PublishJSON(1) + PublishBinary(1) + Publish(1) = 5 writes.
	if w.writes != 5 {
		t.Fatalf("writes = %d, want 5", w.writes)
	}
}

func TestNewProducerEagerlyInitializesWriter(t *testing.T) {
	log := logging.New(&logging.Config{Level: "error"}, "test")
	p, err := NewProducer(
		messaging.Config{Adapter: "kafka", Name: "events"},
		kafka.Config{Brokers: []string{"127.0.0.1:1"}, AllowInsecureDev: true},
		log,
	)
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	if p.writer == nil {
		t.Fatal("eager producer must initialize its writer")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSinkProviderSendRoutesThroughWriter(t *testing.T) {
	w := &fakeKafkaWriter{}
	p := newTestProducer(w, 1, 0)
	p.name = "sink"
	sink := NewSinkProvider("kafka-sink", p)

	if sink.Name() != "kafka-sink" {
		t.Fatalf("Name = %q", sink.Name())
	}
	if !sink.IsAvailable(context.Background()) {
		t.Fatal("expected available sink")
	}
	if sink.Producer() != p {
		t.Fatal("Producer() must return the wrapped producer")
	}
	if err := sink.Send(context.Background(), kafkago.Message{Topic: "orders", Value: []byte("x")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}
}
