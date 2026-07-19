package cascade

import (
	"time"

	"github.com/kbukum/gokit/dag/status"
	"github.com/kbukum/gokit/provider"
)

// CascadeTrace holds execution details for observability.
type CascadeTrace struct {
	StagesExecuted []string                    `json:"stages_executed"`
	StagesSkipped  []string                    `json:"stages_skipped"`
	NodeResults    map[string]CascadeNodeTrace `json:"node_results"`
	TotalDuration  time.Duration               `json:"total_duration"`
	TotalCost      float64                     `json:"total_cost"`
	EarlyExit      bool                        `json:"early_exit"`
	ExitedAtStage  string                      `json:"exited_at_stage,omitempty"`
	Error          error                       `json:"-"`
}

// CascadeNodeTrace holds per-node execution details.
type CascadeNodeTrace struct {
	Name     string        `json:"name"`
	Stage    string        `json:"stage"`
	Duration time.Duration `json:"duration"`
	Status   status.Status `json:"status"`
	Meta     provider.Meta `json:"meta,omitempty"`
	Error    error         `json:"-"`
}

func (c *Cascade[I, O]) computeTotalCost(trace *CascadeTrace) {
	for _, nt := range trace.NodeResults {
		if cost, ok := nt.Meta.Float("cost"); ok {
			trace.TotalCost += cost
		}
	}
}
