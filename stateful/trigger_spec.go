package stateful

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/util"
)

// TriggerKind identifies a declarative flush-trigger variant on the cross-kit wire.
type TriggerKind string

const (
	// TriggerKindSize flushes when the measured size reaches Threshold, counted by the
	// accumulator's Measurer (item count by default).
	TriggerKindSize TriggerKind = "size"
	// TriggerKindByteSize flushes when the measured size reaches Threshold under a byte
	// measurer. gokit's SizeTrigger is measurer-driven, so it builds the same runtime trigger as
	// TriggerKindSize; the kind is preserved on the wire so a byte-oriented configuration keeps
	// its intent and stays interchangeable with the sibling kits, where the unit lives in the
	// trigger.
	TriggerKindByteSize TriggerKind = "byte_size"
	// TriggerKindTime flushes when the time since the last flush reaches Interval.
	TriggerKindTime TriggerKind = "time"
)

// TriggerSpec is a declarative, serde-loadable flush trigger. It is a tagged variant on the wire
// ({"type": <kind>, ...}) so a trigger configuration is interchangeable across kits without
// serializing behavior. Build the concrete Trigger with BuildTrigger.
type TriggerSpec struct {
	// Kind selects the trigger variant.
	Kind TriggerKind
	// Threshold is the size/byte-size threshold; used by the size and byte_size kinds.
	Threshold int
	// Interval is the flush interval; used by the time kind.
	Interval time.Duration
}

// SizeTriggerSpec returns a declarative size trigger that flushes at the given measured threshold.
func SizeTriggerSpec(threshold int) TriggerSpec {
	return TriggerSpec{Kind: TriggerKindSize, Threshold: threshold}
}

// ByteSizeTriggerSpec returns a declarative byte-size trigger that flushes at the given threshold.
func ByteSizeTriggerSpec(threshold int) TriggerSpec {
	return TriggerSpec{Kind: TriggerKindByteSize, Threshold: threshold}
}

// TimeTriggerSpec returns a declarative time trigger that flushes after the given interval.
func TimeTriggerSpec(interval time.Duration) TriggerSpec {
	return TriggerSpec{Kind: TriggerKindTime, Interval: interval}
}

// MarshalJSON encodes the trigger spec as a tagged variant: {"type":"size","threshold":N},
// {"type":"byte_size","threshold":N}, or {"type":"time","interval":"<duration>"}.
func (s TriggerSpec) MarshalJSON() ([]byte, error) {
	switch s.Kind {
	case TriggerKindSize, TriggerKindByteSize:
		return json.Marshal(struct {
			Type      TriggerKind `json:"type"`
			Threshold int         `json:"threshold"`
		}{Type: s.Kind, Threshold: s.Threshold})
	case TriggerKindTime:
		return json.Marshal(struct {
			Type     TriggerKind `json:"type"`
			Interval string      `json:"interval"`
		}{Type: TriggerKindTime, Interval: util.FormatDurationExact(s.Interval)})
	default:
		return nil, fmt.Errorf("stateful: unknown trigger kind %q", s.Kind)
	}
}

// UnmarshalJSON decodes a tagged trigger variant, parsing the interval duration string for the
// time kind and rejecting unknown types.
func (s *TriggerSpec) UnmarshalJSON(data []byte) error {
	var w struct {
		Type      TriggerKind `json:"type"`
		Threshold int         `json:"threshold"`
		Interval  string      `json:"interval"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	switch w.Type {
	case TriggerKindSize, TriggerKindByteSize:
		s.Kind = w.Type
		s.Threshold = w.Threshold
		s.Interval = 0
	case TriggerKindTime:
		d, ok := util.ParseDuration(w.Interval)
		if !ok {
			return fmt.Errorf("stateful: invalid trigger interval %q", w.Interval)
		}
		s.Kind = TriggerKindTime
		s.Threshold = 0
		s.Interval = d
	default:
		return fmt.Errorf("stateful: unknown trigger type %q", w.Type)
	}
	return nil
}

// BuildTrigger instantiates the concrete Trigger described by the spec. The size and byte_size
// kinds both build a measurer-driven SizeTrigger; the byte unit is supplied by configuring a
// ByteSizeMeasurer on the accumulator.
func BuildTrigger[V any](s TriggerSpec) (Trigger[V], error) {
	switch s.Kind {
	case TriggerKindSize, TriggerKindByteSize:
		return SizeTrigger[V](s.Threshold), nil
	case TriggerKindTime:
		return TimeTrigger[V](s.Interval), nil
	default:
		return nil, fmt.Errorf("stateful: unknown trigger kind %q", s.Kind)
	}
}
