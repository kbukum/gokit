package stage

import (
	"context"

	"github.com/kbukum/gokit/stream"
)

// ApplyTransform returns a pipeline that applies t to each item, dropping items the transform does not keep and propagating any transform error. It reuses [stream.FlatMap] so no parallel streaming stack is introduced.
func ApplyTransform[I, O any](p *stream.Pipeline[I], t Transform[I, O]) *stream.Pipeline[O] {
	return stream.FlatMap(p, func(ctx context.Context, in I) (stream.Iterator[O], error) {
		out, keep, err := t.Apply(ctx, in)
		if err != nil {
			return nil, err
		}
		if !keep {
			return stream.FromSlice([]O{}).Iter(ctx), nil
		}
		return stream.FromSlice([]O{out}).Iter(ctx), nil
	})
}
