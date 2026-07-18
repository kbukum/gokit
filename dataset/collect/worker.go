package collect

import (
	"context"

	"github.com/kbukum/gokit/dataset/stage"
	"github.com/kbukum/gokit/stream"
)

// worker pulls sources from workCh, streams each to completion, and publishes a [sourceEvent] on eventCh. It owns its cancellation: it stops when ctx is canceled, so the pool joins cleanly on every exit path.
func (c *Collector[T]) worker(ctx context.Context, workCh <-chan workItem[T], eventCh chan<- sourceEvent[T]) {
	for {
		select {
		case item, ok := <-workCh:
			if !ok {
				return
			}
			select {
			case eventCh <- c.collectSource(ctx, item):
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// collectSource streams one source through the transforms and validator, accumulating its items and real/AI/offset stats onto any resume base. The per-source context is bounded by Config.SourceTimeout when positive.
func (c *Collector[T]) collectSource(ctx context.Context, item workItem[T]) sourceEvent[T] {
	srcCtx := ctx
	if c.config.SourceTimeout > 0 {
		var cancel context.CancelFunc
		srcCtx, cancel = context.WithTimeout(ctx, c.config.SourceTimeout)
		defer cancel()
	}

	_, resumable := item.src.(stage.Resumable)
	ev := sourceEvent[T]{
		index:     item.index,
		name:      item.src.Name(),
		cacheKey:  item.cacheKey,
		resumable: resumable,
	}

	pipeline := item.src.Stream(srcCtx)
	for _, t := range c.transforms {
		pipeline = stage.ApplyTransform(pipeline, t)
	}

	stats := item.resume
	sawOffset := false
	lastOffset := 0
	err := stream.ForEach(srcCtx, pipeline, func(cbCtx context.Context, it T) error {
		if err := cbCtx.Err(); err != nil {
			return err
		}
		if c.validator != nil {
			if err := c.validator.Validate(it); err != nil {
				return err
			}
		}
		if stage.LabelOf(it) == stage.LabelAI {
			stats.AI++
		} else {
			stats.Real++
		}
		stats.Total++
		if off, ok := stage.OffsetOf(it); ok {
			sawOffset = true
			if off > lastOffset {
				lastOffset = off
			}
		}
		ev.items = append(ev.items, it)
		return nil
	})

	if sawOffset {
		stats.FetchedOffset = lastOffset + 1
	} else {
		stats.FetchedOffset = max(stats.Total, item.resume.FetchedOffset)
	}
	ev.stats = stats

	if err != nil {
		ev.outcome = outcomeFailed
		ev.err = err
		return ev
	}
	ev.outcome = outcomeDone
	return ev
}

// streamOf wraps a source's collected items into a pipeline for publishing.
func streamOf[T any](items []T) *stream.Pipeline[T] {
	return stream.FromSlice(items)
}
