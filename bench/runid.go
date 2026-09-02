package bench

import (
	"fmt"

	"github.com/google/uuid"
)

// generateID builds a run identifier of the form <tag|run>_<ts>_<uuid8>: the run tag (or "run"
// when untagged) followed by a compact UTC timestamp and a short unique suffix, so two runs that
// share a tag and second still map to distinct storage files.
func (r *BenchRunner[L]) generateID() string {
	ts := r.cfg.clock.Now().Format("20060102-150405")
	name := r.cfg.tag
	if name == "" {
		name = "run"
	}
	return fmt.Sprintf("%s_%s_%s", name, ts, r.cfg.idSuffix())
}

// randomIDSuffix is the production default suffix source for run IDs.
func randomIDSuffix() string {
	return uuid.New().String()[:8]
}
