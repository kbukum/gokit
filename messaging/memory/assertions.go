package memory

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
)

// AssertPublished fails the test if no message on topic satisfies predicate.
func AssertPublished(t testing.TB, b *InMemoryBroker, topic string, predicate func(messaging.Message) bool) {
	t.Helper()
	for _, msg := range b.Messages(topic) {
		if predicate(msg) {
			return
		}
	}
	t.Errorf("AssertPublished: no message on topic %q matched the predicate (%d checked)", topic, b.MessageCount(topic))
}

// AssertPublishedN fails the test if the number of messages on topic is not exactly n.
func AssertPublishedN(t testing.TB, b *InMemoryBroker, topic string, n int) {
	t.Helper()
	if got := b.MessageCount(topic); got != n {
		t.Errorf("AssertPublishedN: topic %q has %d messages, want %d", topic, got, n)
	}
}

// WaitForMessage blocks until at least one message appears on topic or the timeout expires.
// It returns the first message on the topic.
func WaitForMessage(t testing.TB, b *InMemoryBroker, topic string, timeout time.Duration) messaging.Message {
	t.Helper()

	deadline := time.After(timeout)
	for {
		if msgs := b.Messages(topic); len(msgs) > 0 {
			return msgs[0]
		}
		select {
		case <-deadline:
			t.Fatalf("WaitForMessage: timed out after %v waiting for message on topic %q", timeout, topic)
			return messaging.Message{} // unreachable but keeps the compiler happy
		case <-b.msgCh:
			// A new message was published — re-check.
		}
	}
}

// AssertNoMessages fails the test if any messages were published to the topic.
func AssertNoMessages(t testing.TB, b *InMemoryBroker, topic string) {
	t.Helper()
	if n := b.MessageCount(topic); n != 0 {
		t.Errorf("AssertNoMessages: topic %q has %d messages, want 0", topic, n)
	}
}
