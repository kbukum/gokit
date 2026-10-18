package jetstream

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging/nats"
)

type fakeJS struct {
	natsjs.JetStream
	mu         sync.Mutex
	stream     natsjs.StreamConfig
	consumer   natsjs.ConsumerConfig
	boundTo    string
	bySubject  string
	published  []*natsgo.Msg
	pubOpts    int
	pubErrs    []error
	bindErrs   []error
	pull       *fakeConsumer
	created    natsjs.ConsumerConfig
	deleted    []string
	lastSeq    uint64
	noDeadline bool
	// storeOnCreate simulates a message stored while the replay consumer is created.
	storeOnCreate bool
	looked        string
	binds         int
	declares      int
	// unbounded records an API request made without a deadline.
	unbounded []string
}

func (f *fakeJS) CreateOrUpdateStream(ctx context.Context, cfg natsjs.StreamConfig) (natsjs.Stream, error) {
	f.bounded(ctx, "CreateOrUpdateStream")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stream = cfg
	f.declares++
	return fakeStream{}, nil
}

func (f *fakeJS) StreamNameBySubject(ctx context.Context, subject string) (string, error) {
	f.bounded(ctx, "StreamNameBySubject")
	f.bySubject = subject
	return "FOUND", nil
}

func (f *fakeJS) CreateOrUpdateConsumer(ctx context.Context, stream string, cfg natsjs.ConsumerConfig) (natsjs.Consumer, error) {
	f.bounded(ctx, "CreateOrUpdateConsumer")
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bindErrs) > 0 {
		err := f.bindErrs[0]
		f.bindErrs = f.bindErrs[1:]
		return nil, err
	}
	f.consumer, f.boundTo = cfg, stream
	f.binds++
	return f.pull, nil
}

func (f *fakeJS) PublishMsg(ctx context.Context, msg *natsgo.Msg, opts ...natsjs.PublishOpt) (*natsjs.PubAck, error) {
	if _, ok := ctx.Deadline(); !ok {
		f.noDeadline = true
	}
	f.published, f.pubOpts = append(f.published, msg), len(opts)
	if len(f.pubErrs) > 0 {
		err := f.pubErrs[0]
		f.pubErrs = f.pubErrs[1:]
		return nil, err
	}
	return &natsjs.PubAck{Stream: "FACTS", Sequence: uint64(len(f.published))}, nil
}

func (f *fakeJS) CreateConsumer(ctx context.Context, _ string, cfg natsjs.ConsumerConfig) (natsjs.Consumer, error) {
	f.bounded(ctx, "CreateConsumer")
	f.created = cfg
	if f.storeOnCreate {
		f.lastSeq++
	}
	return f.pull, nil
}

func (f *fakeJS) Consumer(ctx context.Context, stream, durable string) (natsjs.Consumer, error) {
	f.bounded(ctx, "Consumer")
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bindErrs) > 0 {
		err := f.bindErrs[0]
		f.bindErrs = f.bindErrs[1:]
		return nil, err
	}
	f.boundTo, f.looked = stream, durable
	return f.pull, nil
}

func (f *fakeJS) DeleteConsumer(_ context.Context, _ string, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeJS) Stream(ctx context.Context, _ string) (natsjs.Stream, error) {
	f.bounded(ctx, "Stream")
	return fakeStream{last: f.lastSeq}, nil
}

type fakeStream struct {
	natsjs.Stream
	last uint64
}

func (s fakeStream) CachedInfo() *natsjs.StreamInfo {
	return &natsjs.StreamInfo{State: natsjs.StreamState{LastSeq: s.last}}
}

type fakeConsumer struct {
	natsjs.Consumer
	info    natsjs.ConsumerInfo
	fetches []func() (natsjs.MessageBatch, error)
	sizes   []int
	mu      sync.Mutex
}

func (c *fakeConsumer) CachedInfo() *natsjs.ConsumerInfo { return &c.info }

func (c *fakeConsumer) Fetch(n int, _ ...natsjs.FetchOpt) (natsjs.MessageBatch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes = append(c.sizes, n)
	next := c.fetches[0]
	if len(c.fetches) > 1 {
		c.fetches = c.fetches[1:]
	}
	return next()
}

type fakeBatch struct {
	msgs chan natsjs.Msg
	err  error
}

func (b fakeBatch) Messages() <-chan natsjs.Msg { return b.msgs }
func (b fakeBatch) Error() error                { return b.err }

func batchOf(err error, msgs ...natsjs.Msg) natsjs.MessageBatch {
	ch := make(chan natsjs.Msg, len(msgs))
	for _, m := range msgs {
		ch <- m
	}
	close(ch)
	return fakeBatch{msgs: ch, err: err}
}

type fakeMsg struct {
	natsjs.Msg
	seq, delivered uint64
	metaErr        error
	acked          bool
	nakDelay       time.Duration
	naked          bool
	termed         bool
	progressed     bool
}

func (m *fakeMsg) Metadata() (*natsjs.MsgMetadata, error) {
	if m.metaErr != nil {
		return nil, m.metaErr
	}
	return &natsjs.MsgMetadata{
		Stream: "FACTS", Sequence: natsjs.SequencePair{Stream: m.seq}, NumDelivered: m.delivered,
		Timestamp: time.Unix(int64(m.seq), 0), //nolint:gosec // small test sequences
	}, nil
}

func (m *fakeMsg) Headers() natsgo.Header {
	return natsgo.Header{natsgo.MsgIdHdr: {"id-" + strconv.FormatUint(m.seq, 10)}, keyHeader: {"k"}}
}
func (m *fakeMsg) Data() []byte    { return []byte("payload") }
func (m *fakeMsg) Subject() string { return "facts.a" }
func (m *fakeMsg) DoubleAck(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.acked = true
	return nil
}

func (m *fakeMsg) NakWithDelay(d time.Duration) error {
	m.nakDelay = d
	return nil
}

func (m *fakeMsg) Nak() error        { m.naked = true; return nil }
func (m *fakeMsg) Term() error       { m.termed = true; return nil }
func (m *fakeMsg) InProgress() error { m.progressed = true; return nil }

func testClient(js natsjs.JetStream) *Client {
	cfg := nats.Config{URL: "tls://localhost:4222"}
	cfg.ApplyDefaults()
	return &Client{cfg: cfg, js: js}
}

func validStream() StreamConfig {
	return StreamConfig{
		Name: "FACTS", Subjects: []string{"facts.>"}, MaxAge: time.Hour, MaxBytes: 1 << 20, MaxMsgSize: 1024,
		Duplicates: time.Minute,
	}
}

var errTransient = errors.New("transient")

func (f *fakeJS) bounded(ctx context.Context, call string) {
	if _, ok := ctx.Deadline(); !ok {
		f.mu.Lock()
		f.unbounded = append(f.unbounded, call)
		f.mu.Unlock()
	}
}
