package record

import (
	"context"

	"github.com/kbukum/gokit/stream"
)

// Filter returns a pipeline that keeps only the records for which pred returns true.
// It reuses the canonical [stream.Filter] operator.
func Filter(p *stream.Pipeline[Record], pred func(Record) bool) *stream.Pipeline[Record] {
	return stream.Filter(p, pred)
}

// SelectColumns returns a pipeline whose records are projected onto the named columns (missing columns are dropped),
// reusing [stream.Map].
func SelectColumns(p *stream.Pipeline[Record], columns []string) *stream.Pipeline[Record] {
	return stream.Map(p, func(_ context.Context, rec Record) (Record, error) {
		return rec.Select(columns), nil
	})
}
