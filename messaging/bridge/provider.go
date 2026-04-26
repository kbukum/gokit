package bridge

import (
	"context"
	"sync"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/provider"
)

// --- ProducerAsSink ---

// ProducerAsSink wraps a messaging.Producer as a provider.Sink[messaging.Message].
// Messages are published to the given topic using PublishBinary.
func ProducerAsSink(name string, p messaging.Producer, topic string) provider.Sink[messaging.Message] {
	return &producerSink{name: name, producer: p, topic: topic}
}

type producerSink struct {
	name     string
	producer messaging.Producer
	topic    string
}

func (s *producerSink) Name() string                       { return s.name }
func (s *producerSink) IsAvailable(_ context.Context) bool { return s.producer != nil }

func (s *producerSink) Send(ctx context.Context, msg messaging.Message) error {
	return s.producer.PublishBinary(ctx, s.topic, msg.Key, msg.Value)
}

// --- EventProducerAsSink ---

// EventProducerAsSink wraps a messaging.Producer as a provider.Sink[messaging.Event].
// Events are published to the given topic using Publish.
func EventProducerAsSink(name string, p messaging.Producer, topic string) provider.Sink[messaging.Event] {
	return &eventSink{name: name, producer: p, topic: topic}
}

type eventSink struct {
	name     string
	producer messaging.Producer
	topic    string
}

func (s *eventSink) Name() string                       { return s.name }
func (s *eventSink) IsAvailable(_ context.Context) bool { return s.producer != nil }

func (s *eventSink) Send(ctx context.Context, event messaging.Event) error {
	return s.producer.Publish(ctx, s.topic, event)
}

// --- ConsumerAsStream ---

// ConsumerAsStream wraps a messaging.Consumer as a provider.Stream[struct{}, messaging.Message].
// The input parameter is ignored; calling Execute starts the consume loop in a
// background goroutine and returns an iterator that yields each received message.
// Close the iterator to stop the consume loop.
func ConsumerAsStream(name string, c messaging.Consumer) provider.Stream[struct{}, messaging.Message] {
	return &consumerStream{name: name, consumer: c}
}

type consumerStream struct {
	name     string
	consumer messaging.Consumer
}

func (s *consumerStream) Name() string                       { return s.name }
func (s *consumerStream) IsAvailable(_ context.Context) bool { return s.consumer != nil }

func (s *consumerStream) Execute(_ context.Context, _ struct{}) (provider.Iterator[messaging.Message], error) {
	iter := &consumerIterator{
		consumer: s.consumer,
		ch:       make(chan messaging.Message, 64),
		done:     make(chan struct{}),
	}
	iter.start() //nolint:contextcheck // iterator runs as a long-lived bridge goroutine detached from the Execute caller
	return iter, nil
}

// consumerIterator bridges a push-based Consumer into a pull-based Iterator.
type consumerIterator struct {
	consumer messaging.Consumer
	ch       chan messaging.Message
	done     chan struct{}
	cancel   context.CancelFunc
	once     sync.Once
}

func (it *consumerIterator) start() {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is retained on iterator and invoked in Stop()/once-Do
	it.cancel = cancel

	go func() {
		defer close(it.done)
		defer close(it.ch)
		_ = it.consumer.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
			select {
			case it.ch <- msg:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
}

func (it *consumerIterator) Next(ctx context.Context) (messaging.Message, bool, error) {
	select {
	case msg, ok := <-it.ch:
		if !ok {
			return messaging.Message{}, false, nil
		}
		return msg, true, nil
	case <-ctx.Done():
		return messaging.Message{}, false, ctx.Err()
	}
}

func (it *consumerIterator) Close() error {
	it.once.Do(func() {
		it.cancel()
		<-it.done
	})
	return nil
}
