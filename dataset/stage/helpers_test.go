package stage

import "github.com/kbukum/gokit/stream"

// row is a minimal map-backed item used by the stage tests so the low-level
// stage package's tests do not depend on the higher-level record package.
type row = map[string]any

// rowPipeline builds a pipeline of rows for tests.
func rowPipeline(rows ...row) *stream.Pipeline[row] {
	return stream.FromSlice(rows)
}
