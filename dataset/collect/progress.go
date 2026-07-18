package collect

import (
	"github.com/kbukum/gokit/dataset/manifest"
	"github.com/kbukum/gokit/dataset/stage"
)

// Progress observes a [Collector] run. Embed [NullProgress]
// and override only the events you care about.
type Progress interface {
	// OnSourceStart fires before a source is streamed.
	OnSourceStart(index int, name string, maxItems int, hasMax bool)
	// OnSourceProgress fires as items accumulate for a source.
	OnSourceProgress(index int, count int)
	// OnSourceDone fires after a source completes.
	OnSourceDone(index int, name string, stats manifest.SourceStats)
	// OnSourceCached fires when a source is served from the manifest cache.
	OnSourceCached(index int, name string, stats manifest.SourceStats)
	// OnSourceError fires when a source fails.
	OnSourceError(index int, name string, err error)
	// OnPublishDone fires after a target publishes.
	OnPublishDone(target string, result stage.PublishResult)
}

// NullProgress is a no-op [Progress]. Embed it to implement only the events you need.
type NullProgress struct{}

// OnSourceStart does nothing.
func (NullProgress) OnSourceStart(int, string, int, bool) {}

// OnSourceProgress does nothing.
func (NullProgress) OnSourceProgress(int, int) {}

// OnSourceDone does nothing.
func (NullProgress) OnSourceDone(int, string, manifest.SourceStats) {}

// OnSourceCached does nothing.
func (NullProgress) OnSourceCached(int, string, manifest.SourceStats) {}

// OnSourceError does nothing.
func (NullProgress) OnSourceError(int, string, error) {}

// OnPublishDone does nothing.
func (NullProgress) OnPublishDone(string, stage.PublishResult) {}
