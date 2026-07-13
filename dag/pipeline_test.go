package dag

import (
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestScheduleConfig_UnmarshalYAML_SecFields(t *testing.T) {
	yamlData := `interval_sec: 30
min_buffer_sec: 15`

	var sc ScheduleConfig
	if err := yaml.Unmarshal([]byte(yamlData), &sc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if sc.Interval != 30*time.Second {
		t.Fatalf("expected 30s, got %v", sc.Interval)
	}
	if sc.MinBuffer != 15*time.Second {
		t.Fatalf("expected 15s, got %v", sc.MinBuffer)
	}
}

func TestScheduleConfig_UnmarshalYAML_InlineFormat(t *testing.T) {
	// This is the format used in actual pipeline YAML files
	yamlData := `
name: test
nodes:
  - component: ser
    optional: true
    schedule: { interval_sec: 3, min_buffer_sec: 1 }
  - component: sentiment
    schedule: { interval_sec: 30, min_buffer_sec: 15 }
`
	var p Pipeline
	if err := yaml.Unmarshal([]byte(yamlData), &p); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(p.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(p.Nodes))
	}

	// Node 0: ser
	if !p.Nodes[0].Optional {
		t.Fatal("expected ser to be optional")
	}
	if p.Nodes[0].Schedule == nil {
		t.Fatal("expected ser schedule to be non-nil")
	}
	if p.Nodes[0].Schedule.Interval != 3*time.Second {
		t.Fatalf("ser interval: expected 3s, got %v", p.Nodes[0].Schedule.Interval)
	}
	if p.Nodes[0].Schedule.MinBuffer != 1*time.Second {
		t.Fatalf("ser min_buffer: expected 1s, got %v", p.Nodes[0].Schedule.MinBuffer)
	}

	// Node 1: sentiment
	if p.Nodes[1].Schedule.Interval != 30*time.Second {
		t.Fatalf("sentiment interval: expected 30s, got %v", p.Nodes[1].Schedule.Interval)
	}
	if p.Nodes[1].Schedule.MinBuffer != 15*time.Second {
		t.Fatalf("sentiment min_buffer: expected 15s, got %v", p.Nodes[1].Schedule.MinBuffer)
	}
}

func TestScheduleConfig_UnmarshalYAML_FractionalSeconds(t *testing.T) {
	yamlData := `interval_sec: 0.5
min_buffer_sec: 1.5`

	var sc ScheduleConfig
	if err := yaml.Unmarshal([]byte(yamlData), &sc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if sc.Interval != 500*time.Millisecond {
		t.Fatalf("expected 500ms, got %v", sc.Interval)
	}
	if sc.MinBuffer != 1500*time.Millisecond {
		t.Fatalf("expected 1500ms, got %v", sc.MinBuffer)
	}
}

func TestScheduleConfig_UnmarshalYAML_ZeroValues(t *testing.T) {
	yamlData := `interval_sec: 0`

	var sc ScheduleConfig
	if err := yaml.Unmarshal([]byte(yamlData), &sc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if sc.Interval != 0 {
		t.Fatalf("expected 0, got %v", sc.Interval)
	}
	if sc.MinBuffer != 0 {
		t.Fatalf("expected 0 min_buffer, got %v", sc.MinBuffer)
	}
}

func TestScheduleConfig_MarshalYAML_RoundTrip(t *testing.T) {
	original := ScheduleConfig{
		Interval:  30 * time.Second,
		MinBuffer: 15 * time.Second,
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ScheduleConfig
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Interval != original.Interval {
		t.Fatalf("interval round-trip: expected %v, got %v", original.Interval, decoded.Interval)
	}
	if decoded.MinBuffer != original.MinBuffer {
		t.Fatalf("min_buffer round-trip: expected %v, got %v", original.MinBuffer, decoded.MinBuffer)
	}
}

func TestNodeDef_EffectiveOnError(t *testing.T) {
	tests := []struct {
		onError  string
		expected string
	}{
		{"", OnErrorSkip},
		{"skip", OnErrorSkip},
		{"fail", OnErrorFail},
		{"continue", OnErrorContinue},
	}
	for _, tt := range tests {
		def := NodeDef{OnError: tt.onError}
		if got := def.EffectiveOnError(); got != tt.expected {
			t.Errorf("OnError=%q: expected %q, got %q", tt.onError, tt.expected, got)
		}
	}
}

func TestNodeDef_YAMLParsing(t *testing.T) {
	yamlData := `
component: ser
optional: true
on_error: continue
depends_on: [transcription]
schedule: { interval_sec: 5, min_buffer_sec: 2 }
`
	var def NodeDef
	if err := yaml.Unmarshal([]byte(yamlData), &def); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if def.Component != "ser" {
		t.Fatalf("expected 'ser', got %q", def.Component)
	}
	if !def.Optional {
		t.Fatal("expected optional=true")
	}
	if def.OnError != "continue" {
		t.Fatalf("expected on_error='continue', got %q", def.OnError)
	}
	if len(def.DependsOn) != 1 || def.DependsOn[0] != "transcription" {
		t.Fatalf("unexpected depends_on: %v", def.DependsOn)
	}
	if def.Schedule == nil || def.Schedule.Interval != 5*time.Second {
		t.Fatalf("unexpected schedule: %v", def.Schedule)
	}
}
