package stage

import (
	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/stream"
)

// recordPipeline builds a pipeline of records from field maps for tests.
func recordPipeline(recs ...map[string]record.Value) *stream.Pipeline[record.Record] {
	items := make([]record.Record, len(recs))
	for i, r := range recs {
		items[i] = record.New(r)
	}
	return stream.FromSlice(items)
}
