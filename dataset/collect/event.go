package collect

import (
	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/stage"
)

// sourceOutcome classifies how a worker finished streaming one source.
type sourceOutcome int

const (
	// outcomeDone means the source streamed to completion.
	outcomeDone sourceOutcome = iota
	// outcomeFailed means the source errored, timed out, or was canceled.
	outcomeFailed
)

// workItem is one unit of work dispatched to a worker: a source to stream, its cache key,
// and the stats a prior partial run left to resume from.
type workItem[T any] struct {
	index    int
	src      stage.Source[T]
	cacheKey string
	resume   manifest.SourceStats
}

// sourceEvent is the result a worker publishes on the bounded event channel for the main loop to fold into the manifest,
// result, and progress. It carries the source's collected items
// so publishing stays single-owner in the main loop.
type sourceEvent[T any] struct {
	index    int
	name     string
	cacheKey string
	items    []T
	stats    manifest.SourceStats
	outcome  sourceOutcome
	// resumable reports whether a failed source can resume from its partial stats (it implements stage.Resumable).
	resumable bool
	err       error
}
