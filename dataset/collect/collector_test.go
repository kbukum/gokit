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
		WithSources(stage.NewSliceSource("s", []record.Record{
			record.New(map[string]record.Value{"name": "a"}),
			record.New(map[string]record.Value{"name": "b"}),
		})),
		WithTargets(target),
		WithProgress(prog),
		WithClock(newTestClock()),
		WithConfig(Config{OutputDir: dir}),
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

func TestCollectorAppliesTransformAndSchema(t *testing.T) {
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
		WithSources(stage.NewSliceSource("s", []record.Record{
			record.New(map[string]record.Value{"name": "keep"}),
			record.New(map[string]record.Value{"name": "drop"}),
		})),
		WithTransforms(tagTransform("drop")),
		WithSchema(s),
		WithTargets(target),
		WithClock(newTestClock()),
		WithConfig(Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 1 {
		t.Fatalf("TotalItems = %d; want 1 (drop filtered)", res.TotalItems)
	}
}

func TestCollectorSchemaFailsClosed(t *testing.T) {
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
		WithSchema(s),
		WithClock(newTestClock()),
		WithConfig(Config{OutputDir: dir}),
	)
	if _, err := c.Run(context.Background()); err == nil {
		t.Fatal("expected fail-closed schema error")
	}
}

func TestCollectorUsesCacheOnSecondRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	newRun := func(prog Progress) *Collector {
		return New(
			WithSources(stage.NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"name": "a"})})),
			WithProgress(prog),
			WithClock(newTestClock()),
			WithConfig(Config{OutputDir: dir}),
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
	build := func(force bool, prog Progress) *Collector {
		return New(
			WithSources(stage.NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"name": "a"})})),
			WithProgress(prog),
			WithClock(newTestClock()),
			WithConfig(Config{OutputDir: dir, Force: force}),
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
		WithSources(failing),
		WithProgress(prog),
		WithClock(newTestClock()),
		WithConfig(Config{OutputDir: dir}),
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
		WithSources(stage.NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"name": "a"})})),
		WithClock(newTestClock()),
		WithConfig(Config{OutputDir: dir}),
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
		WithSources(stage.NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"name": "a"})})),
		WithClock(&advancingClock{clock: clock, step: 2 * time.Second}),
		WithConfig(Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Duration != 2*time.Second {
		t.Fatalf("Duration = %v; want 2s", res.Duration)
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.OutputDir != DefaultOutputDir || cfg.SourceTimeout != DefaultSourceTimeout {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}
