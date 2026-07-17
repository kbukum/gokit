package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/dataset/schema"
	"github.com/kbukum/gokit/dataset/stage"
)

func TestCollectorRunBasic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := stage.NewSliceTarget[record.Record]("mem")
	prog := &recordingProgress{}
	c := New(
		WithSources(stage.NewSliceSource("s", recordsOf("a", "b"))),
		WithTargets[record.Record](target),
		WithProgress[record.Record](prog),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 2 {
		t.Fatalf("TotalItems = %d; want 2", res.TotalItems)
	}
	if len(target.Records()) != 2 {
		t.Fatalf("target got %d records; want 2", len(target.Records()))
	}
	if prog.started != 1 || prog.done != 1 || prog.publish != 1 {
		t.Fatalf("progress = %+v", prog)
	}
}

func TestCollectorAppliesTransformAndValidator(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := schema.Compile(schema.JSON{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := stage.NewSliceTarget[record.Record]("mem")
	c := New(
		WithSources(stage.NewSliceSource("s", recordsOf("keep", "drop"))),
		WithTransforms(tagTransform("drop")),
		WithValidator(s.Validator()),
		WithTargets[record.Record](target),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 1 {
		t.Fatalf("TotalItems = %d; want 1 (drop filtered)", res.TotalItems)
	}
}

func TestCollectorValidatorFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := schema.Compile(schema.JSON{
		"type":     "object",
		"required": []any{"name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := New(
		WithSources(stage.NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"other": 1})})),
		WithValidator(s.Validator()),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	if _, err := c.Run(context.Background()); err == nil {
		t.Fatal("expected fail-closed validation error")
	}
}

func TestCollectorNilValidatorAcceptsAll(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := New(
		WithSources(stage.NewSliceSource("s", recordsOf("a"))),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 1 {
		t.Fatalf("TotalItems = %d; want 1", res.TotalItems)
	}
}

func TestCollectorUsesCacheOnSecondRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	newRun := func(prog Progress) *Collector[record.Record] {
		return New(
			WithSources(stage.NewSliceSource("s", recordsOf("a"))),
			WithProgress[record.Record](prog),
			WithClock[record.Record](newTestClock()),
			WithConfig[record.Record](Config{OutputDir: dir}),
		)
	}
	if _, err := newRun(NullProgress{}).Run(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	prog := &recordingProgress{}
	res, err := newRun(prog).Run(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if prog.cached != 1 || prog.started != 0 {
		t.Fatalf("second run should be cached: %+v", prog)
	}
	if len(res.CachedSources) != 1 {
		t.Fatalf("CachedSources = %v; want one entry", res.CachedSources)
	}
}

func TestCollectorForceBypassesCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	build := func(force bool, prog Progress) *Collector[record.Record] {
		return New(
			WithSources(stage.NewSliceSource("s", recordsOf("a"))),
			WithProgress[record.Record](prog),
			WithClock[record.Record](newTestClock()),
			WithConfig[record.Record](Config{OutputDir: dir, Force: force}),
		)
	}
	if _, err := build(false, NullProgress{}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	prog := &recordingProgress{}
	if _, err := build(true, prog).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if prog.cached != 0 || prog.started != 1 {
		t.Fatalf("force run should reprocess: %+v", prog)
	}
}

func TestCollectorSourceErrorFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sentinel := errors.New("source boom")
	failing := &funcSource{name: "bad", err: sentinel}
	prog := &recordingProgress{}
	c := New(
		WithSources[record.Record](failing),
		WithProgress[record.Record](prog),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	_, err := c.Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
	if prog.errors != 1 {
		t.Fatalf("expected 1 error callback, got %d", prog.errors)
	}
}

func TestCollectorContextCancelled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(
		WithSources(stage.NewSliceSource("s", recordsOf("a"))),
		WithClock[record.Record](newTestClock()),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	if _, err := c.Run(ctx); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCollectorRecordsDuration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clock := newTestClock()
	c := New(
		WithSources(stage.NewSliceSource("s", recordsOf("a"))),
		WithClock[record.Record](&advancingClock{clock: clock, step: 2 * time.Second}),
		WithConfig[record.Record](Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Duration != 2*time.Second {
		t.Fatalf("Duration = %v; want 2s", res.Duration)
	}
}

func TestCollectorAggregatesRealAIStats(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := &rangeSource{name: "s", total: 10, failAt: -1, ai: func(off int) bool { return off%2 == 1 }}
	c := New(
		WithSources[item](src),
		WithClock[item](newTestClock()),
		WithConfig[item](Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 10 || res.RealItems != 5 || res.AIItems != 5 {
		t.Fatalf("stats = total %d real %d ai %d; want 10/5/5", res.TotalItems, res.RealItems, res.AIItems)
	}
}

func TestCollectorResumesFromPartial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	first := &rangeSource{name: "s", total: 50, failAt: 30}
	c1 := New(
		WithSources[item](first),
		WithClock[item](newTestClock()),
		WithConfig[item](Config{OutputDir: dir}),
	)
	if _, err := c1.Run(context.Background()); !errors.Is(err, errRangeFail) {
		t.Fatalf("first run err = %v; want errRangeFail", err)
	}

	second := &rangeSource{name: "s", total: 50, failAt: -1}
	target := stage.NewSliceTarget[item]("mem")
	c2 := New(
		WithSources[item](second),
		WithTargets[item](target),
		WithClock[item](newTestClock()),
		WithConfig[item](Config{OutputDir: dir}),
	)
	res, err := c2.Run(context.Background())
	if err != nil {
		t.Fatalf("resume run error: %v", err)
	}
	if second.start != 30 {
		t.Fatalf("resumed source start = %d; want 30", second.start)
	}
	if res.SourceStats["s"].Total != 50 {
		t.Fatalf("resumed Total = %d; want 50", res.SourceStats["s"].Total)
	}
	if len(target.Records()) != 20 {
		t.Fatalf("resume published %d items; want 20 new", len(target.Records()))
	}
}

func TestCollectorPerSourceTimeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := New(
		WithSources[item](&blockingSource{name: "slow"}),
		WithClock[item](newTestClock()),
		WithConfig[item](Config{OutputDir: dir, SourceTimeout: 20 * time.Millisecond}),
	)
	_, err := c.Run(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestCollectorBoundedConcurrency(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const n = 3
	bar := newBarrier(n)
	sources := make([]stage.Source[item], n)
	for i := range sources {
		sources[i] = &barrierSource{name: "s", barrier: bar}
	}
	c := New(
		WithSources[item](sources...),
		WithClock[item](newTestClock()),
		WithConfig[item](Config{OutputDir: dir, Concurrency: n}),
	)

	done := make(chan error, 1)
	go func() {
		_, err := c.Run(context.Background())
		done <- err
	}()

	select {
	case <-bar.full:
		close(bar.release)
	case <-time.After(2 * time.Second):
		t.Fatal("expected all sources to stream concurrently")
	}

	if err := <-done; err != nil {
		t.Fatalf("Run error: %v", err)
	}
}
