package stateful

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/util"
)

// AccumulatorSettings describes an accumulator's TTL, capacity, and flush triggers in a
// serde-loadable form that carries no closures or trait objects, so it can be loaded from
// configuration and shared across kits. Call BuildConfig to instantiate the concrete, still
// pluggable Config from the settings; custom triggers, measurers, and handlers remain available
// by setting them on the returned Config in code.
//
// The wire form uses snake_case keys with ttl as a lossless duration string; ttl and max_size
// are omitted when zero, and keep_alive defaults to true when absent on decode.
type AccumulatorSettings struct {
	// TTL is the time-to-live before expiration. Zero means never expire.
	TTL time.Duration
	// KeepAlive enables sliding-window TTL. Defaults to true.
	KeepAlive bool
	// MaxSize is the maximum measured size before FIFO eviction. Zero means unbounded.
	MaxSize int
	// Triggers are the declarative flush triggers evaluated after each append.
	Triggers []TriggerSpec
}

// DefaultAccumulatorSettings returns settings with the cross-kit defaults: no TTL, keep-alive
// enabled, unbounded capacity, and no triggers.
func DefaultAccumulatorSettings() AccumulatorSettings {
	return AccumulatorSettings{KeepAlive: true}
}

type accumulatorSettingsWire struct {
	TTL       string        `json:"ttl,omitempty"`
	KeepAlive bool          `json:"keep_alive"`
	MaxSize   int           `json:"max_size,omitempty"`
	Triggers  []TriggerSpec `json:"triggers"`
}

// MarshalJSON encodes the settings on the cross-kit wire form, emitting TTL as a lossless
// duration string and omitting ttl/max_size when zero.
func (s AccumulatorSettings) MarshalJSON() ([]byte, error) {
	w := accumulatorSettingsWire{
		KeepAlive: s.KeepAlive,
		MaxSize:   s.MaxSize,
		Triggers:  s.Triggers,
	}
	if s.TTL > 0 {
		w.TTL = util.FormatDurationExact(s.TTL)
	}
	if w.Triggers == nil {
		w.Triggers = []TriggerSpec{}
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes the cross-kit wire form, parsing the TTL duration string and defaulting
// keep_alive to true when the key is absent.
func (s *AccumulatorSettings) UnmarshalJSON(data []byte) error {
	w := accumulatorSettingsWire{KeepAlive: true}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	var ttl time.Duration
	if w.TTL != "" {
		d, ok := util.ParseDuration(w.TTL)
		if !ok {
			return fmt.Errorf("stateful: invalid ttl %q", w.TTL)
		}
		ttl = d
	}
	s.TTL = ttl
	s.KeepAlive = w.KeepAlive
	s.MaxSize = w.MaxSize
	s.Triggers = w.Triggers
	return nil
}

// BuildConfig instantiates a Config from the settings, building each declarative TriggerSpec into
// its concrete Trigger. Handlers (OnFlush, OnEvict, ...) and a non-default Measurer are code-only
// concerns and must be set on the returned Config by the caller.
func BuildConfig[V any](s AccumulatorSettings) (Config[V], error) {
	cfg := Config[V]{
		TTL:       s.TTL,
		KeepAlive: s.KeepAlive,
		MaxSize:   s.MaxSize,
	}
	for _, spec := range s.Triggers {
		trigger, err := BuildTrigger[V](spec)
		if err != nil {
			return Config[V]{}, err
		}
		cfg.Triggers = append(cfg.Triggers, trigger)
	}
	return cfg, nil
}
