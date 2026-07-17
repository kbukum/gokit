package collect

import (
	"context"
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
