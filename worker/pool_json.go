package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/util"
)

// poolConfigWire is the cross-kit JSON shape of a PoolConfig: snake_case keys with the grace
// period as a lossless duration string and the dispatch/overflow enums as their snake_case
// values. It mirrors the sibling kits so a pool configuration serialized by any kit decodes in
// the others. Supervisor is a gokit-only extension omitted when absent.
type poolConfigWire struct {
	Name        string            `json:"name"`
	Size        int               `json:"size"`
	QueueSize   int               `json:"queue_size"`
	EventBuffer int               `json:"event_buffer"`
	GracePeriod string            `json:"grace_period"`
	Dispatch    DispatchStrategy  `json:"dispatch"`
	Overflow    OverflowPolicy    `json:"overflow"`
	Supervisor  *SupervisorConfig `json:"supervisor,omitempty"`
}

// MarshalJSON encodes the pool configuration on the cross-kit wire form, emitting GracePeriod as
// a lossless duration string (for example "5s") rather than a nanosecond integer.
func (c PoolConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(poolConfigWire{
		Name:        c.Name,
		Size:        c.Size,
		QueueSize:   c.QueueSize,
		EventBuffer: c.EventBuffer,
		GracePeriod: util.FormatDurationExact(c.GracePeriod),
		Dispatch:    c.Dispatch,
		Overflow:    c.Overflow,
		Supervisor:  c.Supervisor,
	})
}

// UnmarshalJSON decodes the cross-kit wire form produced by MarshalJSON back into a PoolConfig,
// parsing the grace-period duration string. An empty grace period decodes to zero.
func (c *PoolConfig) UnmarshalJSON(data []byte) error {
	var w poolConfigWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	grace := time.Duration(0)
	if w.GracePeriod != "" {
		d, ok := util.ParseDuration(w.GracePeriod)
		if !ok {
			return fmt.Errorf("worker: invalid grace_period %q", w.GracePeriod)
		}
		grace = d
	}
	c.Name = w.Name
	c.Size = w.Size
	c.QueueSize = w.QueueSize
	c.EventBuffer = w.EventBuffer
	c.GracePeriod = grace
	c.Dispatch = w.Dispatch
	c.Overflow = w.Overflow
	c.Supervisor = w.Supervisor
	return nil
}

// supervisorConfigWire is the JSON shape of a SupervisorConfig: snake_case keys with the backoff
// base and health interval as lossless duration strings, keeping the nested configuration
// coherent with the pool-level duration encoding.
type supervisorConfigWire struct {
	RestartPolicy  RestartPolicy `json:"restart_policy"`
	MaxRestarts    int           `json:"max_restarts"`
	BackoffBase    string        `json:"backoff_base"`
	HealthInterval string        `json:"health_interval"`
}

// MarshalJSON encodes the supervisor configuration, emitting its durations as lossless strings.
func (s SupervisorConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(supervisorConfigWire{
		RestartPolicy:  s.RestartPolicy,
		MaxRestarts:    s.MaxRestarts,
		BackoffBase:    util.FormatDurationExact(s.BackoffBase),
		HealthInterval: util.FormatDurationExact(s.HealthInterval),
	})
}

// UnmarshalJSON decodes a supervisor configuration, parsing its duration strings.
func (s *SupervisorConfig) UnmarshalJSON(data []byte) error {
	var w supervisorConfigWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	backoff := time.Duration(0)
	if w.BackoffBase != "" {
		d, ok := util.ParseDuration(w.BackoffBase)
		if !ok {
			return fmt.Errorf("worker: invalid backoff_base %q", w.BackoffBase)
		}
		backoff = d
	}
	health := time.Duration(0)
	if w.HealthInterval != "" {
		d, ok := util.ParseDuration(w.HealthInterval)
		if !ok {
			return fmt.Errorf("worker: invalid health_interval %q", w.HealthInterval)
		}
		health = d
	}
	s.RestartPolicy = w.RestartPolicy
	s.MaxRestarts = w.MaxRestarts
	s.BackoffBase = backoff
	s.HealthInterval = health
	return nil
}
