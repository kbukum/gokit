package jetstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
)

func TestProducerSendMapsMessageAndExpectsStream(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	p, err := testClient(fake).Producer(ProducerConfig{Stream: "FACTS"})
	if err != nil {
		t.Fatal(err)
	}
	msg := messaging.NewMessage("facts.a.b", "run-1", []byte("x"), map[string]string{
		MsgIDHeader: "evt-1", "content-type": "application/x-protobuf",
	})
	if err := p.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	got := fake.published[0]
	if got.Subject != "facts.a.b" || string(got.Data) != "x" || got.Header.Get(MsgIDHeader) != "evt-1" ||
		got.Header.Get(keyHeader) != "run-1" || got.Header.Get("content-type") != "application/x-protobuf" {
		t.Fatalf("published = %+v", got)
	}
	if fake.pubOpts != 1 || fake.noDeadline {
		t.Fatalf("opts=%d noDeadline=%v", fake.pubOpts, fake.noDeadline)
	}
	if _, ok := msg.Headers[keyHeader]; ok {
		t.Fatal("Send mutated the caller's headers")
	}
	if err := p.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProducerPublishVariants(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	p, _ := testClient(fake).Producer(ProducerConfig{})
	ctx := context.Background()
	event := messaging.Event{ID: "evt-9", Type: "run.ended", Source: "runs", Subject: "run-9"}
	if err := p.Publish(ctx, "facts.e", event); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishJSON(ctx, "facts.j", "k", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishBinary(ctx, "facts.b", "", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := p.SendBatch(ctx, []messaging.Message{{Topic: "facts.c"}, {Topic: "facts.d"}}); err != nil {
		t.Fatal(err)
	}
	e, j, b := fake.published[0], fake.published[1], fake.published[2]
	if e.Header.Get(MsgIDHeader) != "evt-9" || e.Header.Get(keyHeader) != "run-9" || e.Header.Get("event-type") != "run.ended" {
		t.Fatalf("event headers = %v", e.Header)
	}
	if j.Header.Get("content-type") != "application/json" || j.Header.Get(keyHeader) != "k" || string(j.Data) != `{"a":1}` {
		t.Fatalf("json = %+v", j)
	}
	if b.Header.Get("content-type") != "application/octet-stream" || b.Header.Get(keyHeader) != "" || len(fake.published) != 5 {
		t.Fatalf("binary = %+v total=%d", b, len(fake.published))
	}
	if err := p.PublishJSON(ctx, "facts.j", "", func() {}); err == nil {
		t.Fatal("unmarshalable value accepted")
	}
}

func TestProducerRefusesInvalidSubjectsAndIDs(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	p, _ := testClient(fake).Producer(ProducerConfig{})
	for _, msg := range []messaging.Message{
		{Topic: "facts.>"},
		{Topic: "facts..a"},
		{Topic: "facts.a", Headers: map[string]string{MsgIDHeader: ""}},
		{Topic: "facts.a", Headers: map[string]string{MsgIDHeader: "has space"}},
		{Topic: "facts.a", Headers: map[string]string{MsgIDHeader: strings.Repeat("x", MaxMessageIDBytes+1)}},
	} {
		if err := p.Send(context.Background(), msg); err == nil {
			t.Fatalf("Send(%+v) accepted invalid message", msg)
		}
	}
	if len(fake.published) != 0 {
		t.Fatal("invalid message reached the broker")
	}
}

func TestProducerRetriesOnlyIdempotentTransientFailures(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	p, _ := testClient(fake).Producer(ProducerConfig{RetryAttempts: 3, RetryBackoff: time.Millisecond})
	ctx := context.Background()
	withID := func(id string) messaging.Message {
		return messaging.Message{Topic: "facts.a", Headers: map[string]string{MsgIDHeader: id}}
	}
	attempts := func(errs []error, send func() error) (int, error) {
		fake.published, fake.pubErrs = nil, errs
		err := send()
		return len(fake.published), err
	}
	if n, err := attempts([]error{errTransient}, func() error { return p.Send(ctx, withID("evt-1")) }); err != nil || n != 2 {
		t.Fatalf("with ID: attempts=%d err=%v", n, err)
	}
	if n, err := attempts([]error{errTransient, errTransient, errTransient}, func() error { return p.Send(ctx, withID("evt-2")) }); !errors.Is(err, errTransient) || n != 3 {
		t.Fatalf("exhausted: attempts=%d err=%v", n, err)
	}
	// Without an ID a lost acknowledgement could store the message twice, so it is attempted once.
	if n, err := attempts([]error{errTransient}, func() error { return p.PublishBinary(ctx, "facts.a", "", nil) }); !errors.Is(err, errTransient) || n != 1 {
		t.Fatalf("without ID: attempts=%d err=%v", n, err)
	}
	mismatch := &natsjs.APIError{Code: 400, ErrorCode: natsjs.ErrorCode(10060), Description: "expected stream does not match"}
	for _, permanentErr := range []error{mismatch, natsgo.ErrMaxPayload} {
		if n, err := attempts([]error{permanentErr}, func() error { return p.Send(ctx, withID("evt-3")) }); !errors.Is(err, permanentErr) || n != 1 {
			t.Fatalf("%v: attempts=%d err=%v", permanentErr, n, err)
		}
	}
}

func TestProducerCanonicalizesTheMessageIDHeader(t *testing.T) {
	t.Parallel()
	fake := &fakeJS{}
	p, _ := testClient(fake).Producer(ProducerConfig{})
	ctx := context.Background()
	headers := map[string]string{"nats-msg-id": "evt-1"}
	if err := p.Send(ctx, messaging.Message{Topic: "facts.a", Headers: headers}); err != nil {
		t.Fatal(err)
	}
	got := fake.published[0].Header
	if got.Get(MsgIDHeader) != "evt-1" || got.Get("nats-msg-id") != "" || headers["nats-msg-id"] != "evt-1" {
		t.Fatalf("headers = %v, caller's = %v", got, headers)
	}
	for _, h := range []map[string]string{
		{"nats-msg-id": "has space"},
		{"nats-msg-id": "a", MsgIDHeader: "b"},
	} {
		if err := p.Send(ctx, messaging.Message{Topic: "facts.a", Headers: h}); err == nil {
			t.Fatalf("Send accepted %v", h)
		}
	}
}

func TestProducerCloseOwnsOnlyItsClient(t *testing.T) {
	t.Parallel()
	shared := testClient(&fakeJS{})
	p, _ := shared.Producer(ProducerConfig{})
	if err := p.Close(); err != nil || p.Close() != nil {
		t.Fatal("close failed")
	}
	if _, err := shared.api(); err != nil {
		t.Fatal("closing a shared producer closed its client")
	}
	if err := p.Flush(context.Background()); !errors.Is(err, messaging.ErrClosed) {
		t.Fatalf("flush after close = %v", err)
	}
	owned := testClient(&fakeJS{})
	op, _ := newProducer(owned.self, ProducerConfig{}, owned)
	if err := op.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := owned.api(); !errors.Is(err, messaging.ErrClosed) {
		t.Fatal("closing an owning producer left its client open")
	}
}
