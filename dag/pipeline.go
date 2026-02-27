package dag

import "time"

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
}

// ScheduleConfig defines schedule-based execution parameters.
type ScheduleConfig struct {
	// Interval is the minimum time between runs.
	Interval time.Duration `yaml:"interval"`
	// MinBuffer is the minimum data accumulation time before first run.
	MinBuffer time.Duration `yaml:"min_buffer"`
}
