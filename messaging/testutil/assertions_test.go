package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

func TestAssertPublished(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	ctx := context.Background()
	_ = p.PublishBinary(ctx, "orders", "k1", []byte("a"))
	_ = p.PublishBinary(ctx, "orders", "k2", []byte("b"))
	_ = p.PublishBinary(ctx, "events", "k3", []byte("c"))

	AssertPublished(t, p, "orders", 2)
	AssertPublished(t, p, "events", 1)
	AssertPublished(t, p, "missing", 0)
}

func TestAssertPublishedN(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	AssertPublishedN(t, p, 0)

	ctx := context.Background()
	_ = p.PublishBinary(ctx, "t", "k", []byte("v"))
	AssertPublishedN(t, p, 1)
}

func TestAssertConsumed(t *testing.T) {
	t.Parallel()

	received := []messaging.Message{
		{Key: "k1", Value: []byte("v1")},
		{Key: "k2", Value: []byte("v2")},
	}
	AssertConsumed(t, received, 2)
	AssertConsumed(t, nil, 0)
}

func TestAssertNoMessages(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	AssertNoMessages(t, p)
}
