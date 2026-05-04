package messaging

import (
	"context"
	"errors"
	"testing"
)

// adapterProducer is a minimal Producer implementation for adapter tests.
type adapterProducer struct {
	publishBinaryCalls []adapterPublishCall
	publishBinaryErr   error
	closeCalled        bool
}

type adapterPublishCall struct {
	topic string
	key   string
	data  []byte
}

func (p *adapterProducer) Publish(_ context.Context, _ string, _ Event, _ ...string) error {
	return nil
}

func (p *adapterProducer) PublishJSON(_ context.Context, _, _ string, _ any) error {
	return nil
}

func (p *adapterProducer) PublishBinary(_ context.Context, topic, key string, data []byte) error {
	p.publishBinaryCalls = append(p.publishBinaryCalls, adapterPublishCall{topic, key, data})
	return p.publishBinaryErr
}

func (p *adapterProducer) Send(ctx context.Context, msg Message) error {
	return p.PublishBinary(ctx, msg.Topic, msg.Key, msg.Value)
}

func (p *adapterProducer) SendBatch(ctx context.Context, messages []Message) error {
	for _, msg := range messages {
		if err := p.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
func (p *adapterProducer) Flush(context.Context) error { return nil }
func (p *adapterProducer) Close() error {
	p.closeCalled = true
	return nil
}

// --- ConsumerProviderAdapter tests ---

func TestConsumerProviderAdapter_Name(t *testing.T) {
	sc := &stubConsumer{topic: "t"}
	a := NewConsumerProviderAdapter("my-consumer", sc)
	if got := a.Name(); got != "my-consumer" {
		t.Fatalf("Name() = %q, want %q", got, "my-consumer")
	}
}

func TestConsumerProviderAdapter_IsAvailable(t *testing.T) {
	sc := &stubConsumer{topic: "t"}
	a := NewConsumerProviderAdapter("c", sc)
	if !a.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=true with non-nil consumer")
	}

	nilAdapter := NewConsumerProviderAdapter("c", nil)
	if nilAdapter.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=false with nil consumer")
	}
}

// --- ProducerProviderAdapter tests ---

func TestProducerProviderAdapter_Name(t *testing.T) {
	p := &adapterProducer{}
	a := NewProducerProviderAdapter("my-producer", p)
	if got := a.Name(); got != "my-producer" {
		t.Fatalf("Name() = %q, want %q", got, "my-producer")
	}
}

func TestProducerProviderAdapter_IsAvailable(t *testing.T) {
	p := &adapterProducer{}
	a := NewProducerProviderAdapter("p", p)
	if !a.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=true with non-nil producer")
	}

	nilAdapter := NewProducerProviderAdapter("p", nil)
	if nilAdapter.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=false with nil producer")
	}
}

func TestProducerProviderAdapter_Send(t *testing.T) {
	p := &adapterProducer{}
	a := NewProducerProviderAdapter("p", p)

	msg := Message{
		Topic: "send-topic",
		Key:   "msg-key",
		Value: []byte("hello"),
	}

	if err := a.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if len(p.publishBinaryCalls) != 1 {
		t.Fatalf("expected 1 PublishBinary call, got %d", len(p.publishBinaryCalls))
	}

	call := p.publishBinaryCalls[0]
	if call.topic != "send-topic" {
		t.Errorf("topic = %q, want %q", call.topic, "send-topic")
	}
	if call.key != "msg-key" {
		t.Errorf("key = %q, want %q", call.key, "msg-key")
	}
	if string(call.data) != "hello" {
		t.Errorf("data = %q, want %q", call.data, "hello")
	}
}

func TestProducerProviderAdapter_Send_Error(t *testing.T) {
	wantErr := errors.New("publish failed")
	p := &adapterProducer{publishBinaryErr: wantErr}
	a := NewProducerProviderAdapter("p", p)

	msg := Message{Topic: "t", Key: "k", Value: []byte("v")}
	if err := a.Send(context.Background(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("Send error = %v, want %v", err, wantErr)
	}
}

func TestProducerProviderAdapter_Producer(t *testing.T) {
	p := &adapterProducer{}
	a := NewProducerProviderAdapter("p", p)
	if a.Producer() != p {
		t.Fatal("Producer() did not return the underlying producer")
	}
}

// --- ProducerSinkProvider tests ---

func TestProducerSinkProvider_Name(t *testing.T) {
	p := &adapterProducer{}
	s := NewProducerSinkProvider("my-sink", p)
	if got := s.Name(); got != "my-sink" {
		t.Fatalf("Name() = %q, want %q", got, "my-sink")
	}
}

func TestProducerSinkProvider_IsAvailable(t *testing.T) {
	p := &adapterProducer{}
	s := NewProducerSinkProvider("s", p)
	if !s.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=true with non-nil producer")
	}

	nilSink := NewProducerSinkProvider("s", nil)
	if nilSink.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable=false with nil producer")
	}
}

func TestProducerSinkProvider_Send(t *testing.T) {
	p := &adapterProducer{}
	s := NewProducerSinkProvider("s", p)

	msg := Message{
		Topic: "sink-topic",
		Key:   "sink-key",
		Value: []byte("sink-data"),
	}

	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if len(p.publishBinaryCalls) != 1 {
		t.Fatalf("expected 1 PublishBinary call, got %d", len(p.publishBinaryCalls))
	}

	call := p.publishBinaryCalls[0]
	if call.topic != "sink-topic" {
		t.Errorf("topic = %q, want %q", call.topic, "sink-topic")
	}
	if call.key != "sink-key" {
		t.Errorf("key = %q, want %q", call.key, "sink-key")
	}
	if string(call.data) != "sink-data" {
		t.Errorf("data = %q, want %q", call.data, "sink-data")
	}
}

func TestProducerSinkProvider_Send_Error(t *testing.T) {
	wantErr := errors.New("sink publish failed")
	p := &adapterProducer{publishBinaryErr: wantErr}
	s := NewProducerSinkProvider("s", p)

	msg := Message{Topic: "t", Key: "k", Value: []byte("v")}
	if err := s.Send(context.Background(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("Send error = %v, want %v", err, wantErr)
	}
}

func TestProducerSinkProvider_Producer(t *testing.T) {
	p := &adapterProducer{}
	s := NewProducerSinkProvider("s", p)
	if s.Producer() != p {
		t.Fatal("Producer() did not return the underlying producer")
	}
}
