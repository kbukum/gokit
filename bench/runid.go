package bench

import (
	"fmt"

	"github.com/google/uuid"
)

func (r *BenchRunner[L]) generateID() string {
	ts := r.cfg.clock.Now().Format("20060102-150405")
	if r.cfg.tag != "" {
		return fmt.Sprintf("%s_%s", r.cfg.tag, ts)
	}
	return fmt.Sprintf("run_%s_%s", ts, r.cfg.idSuffix())
}

// randomIDSuffix is the production default suffix source for untagged run IDs.
func randomIDSuffix() string {
	return uuid.New().String()[:8]
}
