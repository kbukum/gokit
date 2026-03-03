package dag

import (
	"errors"
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

// Pipeline is a composable, YAML-defined graph definition.
type Pipeline struct {
	// Name is the pipeline identifier.
	Name string `yaml:"name"`
	// Mode is the execution mode: "batch" or "streaming".
	Mode string `yaml:"mode"`
	// Includes lists sub-pipeline names to compose (recursive).
	Includes []string `yaml:"includes,omitempty"`
	// Nodes defines the pipeline's node specifications.
	Nodes []NodeDef `yaml:"nodes"`
}

// Error policy constants for NodeDef.OnError.
const (
	// OnErrorSkip skips dependents when this node fails or is unavailable (default).
	OnErrorSkip = "skip"
	// OnErrorFail halts the entire pipeline when this node fails.
	OnErrorFail = "fail"
	// OnErrorContinue runs dependents regardless of this node's failure.
	OnErrorContinue = "continue"
)

// NodeDef defines a node within a pipeline.
type NodeDef struct {
	// Component is the registry lookup key for this node.
	Component string `yaml:"component"`
	// DependsOn lists node names this node depends on.
	DependsOn []string `yaml:"depends_on,omitempty"`
	// Schedule configures schedule-based execution (streaming mode only).
	Schedule *ScheduleConfig `yaml:"schedule,omitempty"`
	// Condition is a named condition function key.
	Condition string `yaml:"condition,omitempty"`
	// Optional marks the node as optional. When true and the component is not
	// in the registry, a placeholder node is inserted that returns ErrUnavailable.
	// The node stays in the graph and is re-evaluated every execution cycle.
	Optional bool `yaml:"optional,omitempty"`
	// OnError controls how failures propagate to dependents.
	// "skip" (default): dependents are skipped this cycle.
	// "continue": dependents run regardless.
	// "fail": halt the entire pipeline.
	OnError string `yaml:"on_error,omitempty"`
}

// EffectiveOnError returns the on_error policy, defaulting to OnErrorSkip.
func (d NodeDef) EffectiveOnError() string {
	if d.OnError == "" {
		return OnErrorSkip
	}
	return d.OnError
}

// ScheduleConfig defines schedule-based execution parameters.
// YAML format uses integer seconds: { interval_sec: 30, min_buffer_sec: 15 }
type ScheduleConfig struct {
	// Interval is the minimum time between runs.
	Interval time.Duration
	// MinBuffer is the minimum data accumulation time before first run.
	MinBuffer time.Duration
}

// scheduleYAML is the YAML representation with integer seconds.
type scheduleYAML struct {
	IntervalSec  *float64 `yaml:"interval_sec"`
	MinBufferSec *float64 `yaml:"min_buffer_sec"`
	// Legacy fields for programmatic use (time.Duration nanoseconds)
	Interval  *time.Duration `yaml:"interval"`
	MinBuffer *time.Duration `yaml:"min_buffer"`
}

// UnmarshalYAML reads ScheduleConfig from YAML.
// Supports both { interval_sec: 30 } and { interval: 30s } formats.
func (s *ScheduleConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw scheduleYAML
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("dag: parsing schedule: %w", err)
	}

	// Prefer _sec fields (YAML-friendly integer seconds)
	if raw.IntervalSec != nil {
		s.Interval = time.Duration(*raw.IntervalSec * float64(time.Second))
	} else if raw.Interval != nil {
		s.Interval = *raw.Interval
	}

	if raw.MinBufferSec != nil {
		s.MinBuffer = time.Duration(*raw.MinBufferSec * float64(time.Second))
	} else if raw.MinBuffer != nil {
		s.MinBuffer = *raw.MinBuffer
	}

	return nil
}

// MarshalYAML writes ScheduleConfig as YAML with integer seconds.
func (s ScheduleConfig) MarshalYAML() (interface{}, error) {
	out := make(map[string]interface{})
	if s.Interval > 0 {
		out["interval_sec"] = s.Interval.Seconds()
	}
	if s.MinBuffer > 0 {
		out["min_buffer_sec"] = s.MinBuffer.Seconds()
	}
	return out, nil
}

// ErrUnavailable is returned by placeholder nodes for optional components
// that are not registered. The engine uses this to distinguish "node is not
// available right now" from "node failed with an error".
var ErrUnavailable = errors.New("dag: node unavailable")
