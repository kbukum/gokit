package collect

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/dataset/stage"
	"github.com/kbukum/gokit/stream"
	"github.com/kbukum/gokit/util"
)

// errIter is a stream iterator that fails on first pull, used to simulate a
// source that errors mid-collection.
type errIter struct {
	err error
}

func (it *errIter) Next(context.Context) (record.Record, bool, error) {
	return record.Record{}, false, it.err
}
func (it *errIter) Close() error { return nil }

// funcSource is a test [stage.Source] whose stream immediately yields err.
type funcSource struct {
	name string
	err  error
}

func (s *funcSource) Name() string { return s.name }

func (s *funcSource) Stream(context.Context) *stream.Pipeline[record.Record] {
	return stream.FromFunc(func(context.Context) stream.Iterator[record.Record] {
		return &errIter{err: s.err}
	})
}

// advancingClock advances a FakeClock by step on every Now call so run timing
// is deterministic yet non-zero.
type advancingClock struct {
	clock *util.FakeClock
	step  time.Duration
}

func (c *advancingClock) Now() time.Time {
	now := c.clock.Now()
	c.clock.Advance(c.step)
	return now
}

func newTestClock() *util.FakeClock {
	return util.NewFakeClock(time.Unix(0, 0).UTC())
}

// tagTransform drops the record whose name equals drop and tags the rest.
func tagTransform(drop string) stage.Transform[record.Record, record.Record] {
	return stage.TransformFunc[record.Record, record.Record]{
		FuncName: "tag",
		Fn: func(_ context.Context, in record.Record) (record.Record, bool, error) {
			if v, _ := in.Get("name"); v == drop {
				return record.Record{}, false, nil
			}
			fields := in.Fields()
			fields["tagged"] = true
			return record.New(fields), true, nil
		},
	}
}

// recordingProgress counts the collector's progress events.
type recordingProgress struct {
	NullProgress
	started int
	done    int
	cached  int
	errors  int
	publish int
}

func (r *recordingProgress) OnSourceStart(int, string, int, bool)             { r.started++ }
func (r *recordingProgress) OnSourceDone(int, string, manifest.SourceStats)   { r.done++ }
func (r *recordingProgress) OnSourceCached(int, string, manifest.SourceStats) { r.cached++ }
func (r *recordingProgress) OnSourceError(int, string, error)                 { r.errors++ }
func (r *recordingProgress) OnPublishDone(string, stage.PublishResult)        { r.publish++ }

func recordsOf(names ...string) []record.Record {
	recs := make([]record.Record, len(names))
	for i, n := range names {
		recs[i] = record.New(map[string]record.Value{"name": n})
	}
	return recs
}

// item is a test item carrying an explicit real/AI label and source offset so
// stat aggregation and offset resume can be exercised.
type item struct {
	offset int
	ai     bool
}

func (i item) Label() stage.Label {
	if i.ai {
		return stage.LabelAI
	}
	return stage.LabelReal
}

func (i item) SourceOffset() (int, bool) { return i.offset, true }

// rangeSource emits items at offsets [start, total). When failAt is
// non-negative it errors on reaching that offset, simulating a partial run.
// It is [stage.Resumable]: a resumed run advances start past the fetched offset.
type rangeSource struct {
	name   string
	total  int
	failAt int
	start  int
	ai     func(offset int) bool
}

func (s *rangeSource) Name() string            { return s.name }
func (s *rangeSource) MaxItems() (int, bool)   { return s.total, true }
func (s *rangeSource) SetResumeState(o, _ int) { s.start = o }

func (s *rangeSource) Stream(context.Context) *stream.Pipeline[item] {
	return stream.FromFunc(func(context.Context) stream.Iterator[item] {
		return &rangeIter{cur: s.start, total: s.total, failAt: s.failAt, ai: s.ai}
	})
}

type rangeIter struct {
	cur    int
	total  int
	failAt int
	ai     func(int) bool
}

func (it *rangeIter) Next(context.Context) (item, bool, error) {
	if it.failAt >= 0 && it.cur == it.failAt {
		return item{}, false, errRangeFail
	}
	if it.cur >= it.total {
		return item{}, false, nil
	}
	off := it.cur
	it.cur++
	ai := it.ai != nil && it.ai(off)
	return item{offset: off, ai: ai}, true, nil
}

func (it *rangeIter) Close() error { return nil }

var errRangeFail = errors.New("range source failed")

// barrierSource blocks in its stream factory until the shared barrier releases,
// proving how many sources stream concurrently.
type barrierSource struct {
	name    string
	barrier *barrier
}

func (s *barrierSource) Name() string          { return s.name }
func (s *barrierSource) MaxItems() (int, bool) { return 1, true }

func (s *barrierSource) Stream(ctx context.Context) *stream.Pipeline[item] {
	return stream.FromFunc(func(context.Context) stream.Iterator[item] {
		s.barrier.arriveAndWait(ctx)
		return &emptyIter{}
	})
}

// emptyIter yields no items.
type emptyIter struct{}

func (it *emptyIter) Next(context.Context) (item, bool, error) { return item{}, false, nil }
func (it *emptyIter) Close() error                             { return nil }

// barrier releases all waiters only once n of them have arrived.
type barrier struct {
	n       int
	mu      sync.Mutex
	arrived int
	release chan struct{}
	full    chan struct{}
}

func newBarrier(n int) *barrier {
	return &barrier{n: n, release: make(chan struct{}), full: make(chan struct{})}
}

func (b *barrier) arriveAndWait(ctx context.Context) {
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.n {
		close(b.full)
	}
	b.mu.Unlock()
	select {
	case <-b.release:
	case <-ctx.Done():
	}
}

// blockingSource blocks forever until its context is canceled, exercising the
// per-source timeout.
type blockingSource struct{ name string }

func (s *blockingSource) Name() string          { return s.name }
func (s *blockingSource) MaxItems() (int, bool) { return 1, true }

func (s *blockingSource) Stream(context.Context) *stream.Pipeline[item] {
	return stream.FromFunc(func(context.Context) stream.Iterator[item] {
		return &blockingIter{}
	})
}

type blockingIter struct{}

func (it *blockingIter) Next(ctx context.Context) (item, bool, error) {
	<-ctx.Done()
	return item{}, false, ctx.Err()
}

func (it *blockingIter) Close() error { return nil }
