package testutil

import (
	"testing"

	"github.com/kbukum/gokit/messaging"
)

// AssertPublished asserts that exactly count messages were published to the given topic. It reports test failures with the topic name and counts.
func AssertPublished(t *testing.T, p *MockProducer, topic string, count int) {
	t.Helper()
	msgs := p.MessagesForTopic(topic)
	if len(msgs) != count {
		t.Errorf("expected %d messages on topic %q, got %d", count, topic, len(msgs))
	}
}

// AssertPublishedN asserts that exactly count messages were published in total, regardless of topic.
func AssertPublishedN(t *testing.T, p *MockProducer, count int) {
	t.Helper()
	msgs := p.Messages()
	if len(msgs) != count {
		t.Errorf("expected %d total messages, got %d", count, len(msgs))
	}
}

// AssertConsumed asserts that exactly count messages are present in the received slice. Use this with a handler that collects messages:
//
//	var received []messaging.Message
//	handler := func(_ context.Context, msg messaging.Message) error {
//	    received = append(received, msg)
//	    return nil
//	}
func AssertConsumed(t *testing.T, received []messaging.Message, count int) {
	t.Helper()
	if len(received) != count {
		t.Errorf("expected %d consumed messages, got %d", count, len(received))
	}
}

// AssertNoMessages asserts that the producer recorded zero messages.
func AssertNoMessages(t *testing.T, p *MockProducer) {
	t.Helper()
	if p.HasMessages() {
		t.Errorf("expected no messages, got %d", p.MessageCount())
	}
}
