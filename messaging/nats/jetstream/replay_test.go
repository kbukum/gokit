package jetstream

import (
	"context"
	"testing"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

func TestReplayPagesAndDeletesConsumer(t *testing.T) {
	t.Parallel()
	pull := &fakeConsumer{info: natsjs.ConsumerInfo{Name: "replay-1", NumPending: 5}}
	pull.fetches = []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) {
			return batchOf(nil, &fakeMsg{seq: 7, delivered: 1}, &fakeMsg{seq: 9, delivered: 1}), nil
		},
	}
	fake := &fakeJS{pull: pull}
	page, err := testClient(fake).Replay(context.Background(), ReplayRequest{
		Stream: "FACTS", Subject: "facts.a.>", StartSequence: 7, Limit: 2, Wait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[1].Offset != 9 || page.Next != 10 || !page.More || pull.sizes[0] != 2 {
		t.Fatalf("page = %+v sizes=%v", page, pull.sizes)
	}
	c := fake.created
	if c.DeliverPolicy != natsjs.DeliverByStartSequencePolicy || c.OptStartSeq != 7 || c.AckPolicy != natsjs.AckNonePolicy ||
		c.FilterSubjects[0] != "facts.a.>" || c.Durable != "" || c.InactiveThreshold <= 0 {
		t.Fatalf("replay consumer = %+v", c)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "replay-1" {
		t.Fatalf("deleted = %v", fake.deleted)
	}
}

func TestReplayEmptyPageKeepsCheckpoint(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{pull: &fakeConsumer{info: natsjs.ConsumerInfo{Name: "r"}}, lastSeq: 41}
	c := testClient(fake)
	page, err := c.Replay(context.Background(), ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartSequence: 12, Limit: 5, Wait: time.Second})
	if err != nil || page.Next != 12 || page.More || len(page.Messages) != 0 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	fake.storeOnCreate = true
	page, err = c.Replay(context.Background(), ReplayRequest{Stream: "FACTS", Subject: "facts.>", StartTime: time.Unix(1, 0), Limit: 5, Wait: time.Second})
	// A message stored while the consumer was created (sequence 42) must not be skipped: the page resumes at 42.
	if err != nil || page.Next != 42 || fake.created.DeliverPolicy != natsjs.DeliverByStartTimePolicy {
		t.Fatalf("time page=%+v err=%v", page, err)
	}
}

func TestReplayFailsShortPage(t *testing.T) {
	t.Parallel()
	pull := &fakeConsumer{info: natsjs.ConsumerInfo{Name: "r", NumPending: 3}}
	pull.fetches = []func() (natsjs.MessageBatch, error){
		func() (natsjs.MessageBatch, error) { return batchOf(nil, &fakeMsg{seq: 1}), nil },
	}
	if _, err := testClient(&fakeJS{pull: pull}).Replay(context.Background(), ReplayRequest{
		Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: 3, Wait: time.Millisecond,
	}); err == nil {
		t.Fatal("expected short page error")
	}
}

func TestReplayRequestValidate(t *testing.T) {
	t.Parallel()
	for _, r := range []ReplayRequest{
		{Stream: "FACTS", Subject: "facts.>", Limit: 1, Wait: time.Second},
		{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, StartTime: time.Unix(1, 0), Limit: 1, Wait: time.Second},
		{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: MaxReplayLimit + 1, Wait: time.Second},
		{Stream: "FACTS", Subject: "facts.>", StartSequence: 1, Limit: 1},
		{Stream: "FACTS", Subject: "", StartSequence: 1, Limit: 1, Wait: time.Second},
		{Stream: "F.X", Subject: "facts.>", StartSequence: 1, Limit: 1, Wait: time.Second},
	} {
		if err := r.Validate(); err == nil {
			t.Fatalf("Validate(%+v) accepted invalid request", r)
		}
	}
}
