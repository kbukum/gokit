package dag

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// =============================================================================
// ScheduleConfig YAML Tests
// =============================================================================

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

// =============================================================================
// NodeDef Tests
// =============================================================================

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

// =============================================================================
// UnavailableNode Tests
// =============================================================================

func TestUnavailableNode_ReturnsErrUnavailable(t *testing.T) {
	node := NewUnavailableNode("ser")
	if node.Name() != "ser" {
		t.Fatalf("expected name 'ser', got %q", node.Name())
	}

	_, err := node.Run(context.Background(), NewState())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

// =============================================================================
// ResolvePipeline — Optional Node Tests
// =============================================================================

func TestResolvePipeline_OptionalMissing_PlaceholderInserted(t *testing.T) {
	reg := NewRegistry()
	reg.Register("transcription", newFuncNode("transcription", nil))
	// "ser" is NOT registered

	p := &Pipeline{
		Name: "test",
		Nodes: []NodeDef{
			{Component: "transcription"},
			{Component: "ser", Optional: true, DependsOn: []string{"transcription"}},
		},
	}

	g, err := ResolvePipeline(p, reg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both nodes should be in the graph
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}

	// ser should be an unavailableNode
	_, runErr := g.Nodes["ser"].Run(context.Background(), NewState())
	if !errors.Is(runErr, ErrUnavailable) {
		t.Fatalf("expected ser to return ErrUnavailable, got %v", runErr)
	}

	// NodeDefs should be stored
	if !g.NodeDefs["ser"].Optional {
		t.Fatal("expected ser NodeDef to have Optional=true")
	}
}

func TestResolvePipeline_OptionalPresent_NormalResolution(t *testing.T) {
	reg := NewRegistry()
	reg.Register("ser", newFuncNode("ser", func(_ context.Context, s *State) (any, error) {
		return "ser-output", nil
	}))

	p := &Pipeline{
		Name: "test",
		Nodes: []NodeDef{
			{Component: "ser", Optional: true},
		},
	}

	g, err := ResolvePipeline(p, reg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the real node, not placeholder
	out, runErr := g.Nodes["ser"].Run(context.Background(), NewState())
	if runErr != nil {
		t.Fatalf("expected no error, got %v", runErr)
	}
	if out != "ser-output" {
		t.Fatalf("expected 'ser-output', got %v", out)
	}
}

func TestResolvePipeline_RequiredMissing_Error(t *testing.T) {
	reg := NewRegistry()
	p := &Pipeline{
		Name:  "test",
		Nodes: []NodeDef{{Component: "missing"}},
	}

	_, err := ResolvePipeline(p, reg, nil)
	if err == nil {
		t.Fatal("expected error for missing required component")
	}
}

func TestResolvePipeline_OptionalWithIncludes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", newFuncNode("a", nil))
	// "b" is NOT registered

	sub := &Pipeline{
		Name: "sub",
		Nodes: []NodeDef{
			{Component: "b", Optional: true},
		},
	}

	main := &Pipeline{
		Name:     "main",
		Includes: []string{"sub"},
		Nodes: []NodeDef{
			{Component: "a"},
		},
	}

	loader := &memoryLoader{pipelines: map[string]*Pipeline{"sub": sub}}
	g, err := ResolvePipeline(main, reg, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both nodes should be present
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}

	// b should be unavailable
	_, runErr := g.Nodes["b"].Run(context.Background(), NewState())
	if !errors.Is(runErr, ErrUnavailable) {
		t.Fatalf("expected b to return ErrUnavailable, got %v", runErr)
	}
}

func TestResolvePipeline_NodeDefsStored(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", newFuncNode("a", nil))

	p := &Pipeline{
		Name: "test",
		Nodes: []NodeDef{
			{Component: "a", OnError: "continue"},
		},
	}

	g, err := ResolvePipeline(p, reg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def := g.GetNodeDef("a")
	if def.OnError != "continue" {
		t.Fatalf("expected on_error='continue', got %q", def.OnError)
	}
}

// =============================================================================
// Engine — Dependency Propagation Tests
// =============================================================================

func TestEngine_UnavailableSkipsDependents(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"ser":         NewUnavailableNode("ser"),
			"compositor":  newFuncNode("compositor", func(_ context.Context, _ *State) (any, error) { return "c", nil }),
			"independent": newFuncNode("independent", func(_ context.Context, _ *State) (any, error) { return "i", nil }),
		},
		Edges: []Edge{
			{From: "ser", To: "compositor"},
		},
		NodeDefs: map[string]NodeDef{
			"ser":         {Component: "ser", Optional: true},
			"compositor":  {Component: "compositor"},
			"independent": {Component: "independent"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ser should be unavailable
	if result.NodeResults["ser"].Status != StatusUnavailable {
		t.Fatalf("expected ser=%s, got %s", StatusUnavailable, result.NodeResults["ser"].Status)
	}
	// compositor should be skipped due to dep unavailable
	if result.NodeResults["compositor"].Status != StatusDepUnavailable {
		t.Fatalf("expected compositor=%s, got %s", StatusDepUnavailable, result.NodeResults["compositor"].Status)
	}
	// independent should complete normally
	if result.NodeResults["independent"].Status != StatusCompleted {
		t.Fatalf("expected independent=%s, got %s", StatusCompleted, result.NodeResults["independent"].Status)
	}
}

func TestEngine_FailedSkipsDependents(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, fmt.Errorf("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-result", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a"},
			"b": {Component: "b"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("expected a=%s, got %s", StatusFailed, result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != StatusDepFailed {
		t.Fatalf("expected b=%s, got %s", StatusDepFailed, result.NodeResults["b"].Status)
	}
}

func TestEngine_OnErrorContinue_RunsDependents(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, fmt.Errorf("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-ran", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: "continue"}, // on_error=continue on upstream
			"b": {Component: "b"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("expected a=%s, got %s", StatusFailed, result.NodeResults["a"].Status)
	}
	// b should run because a has on_error=continue
	if result.NodeResults["b"].Status != StatusCompleted {
		t.Fatalf("expected b=%s, got %s", StatusCompleted, result.NodeResults["b"].Status)
	}
	if result.NodeResults["b"].Output != "b-ran" {
		t.Fatalf("expected b output='b-ran', got %v", result.NodeResults["b"].Output)
	}
}

func TestEngine_OnErrorFail_HaltsPipeline(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, fmt.Errorf("critical failure")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-result", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: "fail"},
			"b": {Component: "b"},
		},
	}

	engine := &Engine{}
	_, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err == nil {
		t.Fatal("expected pipeline halt error")
	}
}

func TestEngine_MultiLevelCascade(t *testing.T) {
	// a → b → c → d, a is unavailable → b,c,d all skipped
	g := &Graph{
		Nodes: map[string]Node{
			"a": NewUnavailableNode("a"),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) { return "b", nil }),
			"c": newFuncNode("c", func(_ context.Context, _ *State) (any, error) { return "c", nil }),
			"d": newFuncNode("d", func(_ context.Context, _ *State) (any, error) { return "d", nil }),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "d"},
		},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", Optional: true},
			"b": {Component: "b"},
			"c": {Component: "c"},
			"d": {Component: "d"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusUnavailable {
		t.Fatalf("expected a=%s, got %s", StatusUnavailable, result.NodeResults["a"].Status)
	}
	for _, name := range []string{"b", "c", "d"} {
		if result.NodeResults[name].Status != StatusDepUnavailable {
			t.Fatalf("expected %s=%s, got %s", name, StatusDepUnavailable, result.NodeResults[name].Status)
		}
	}
}

func TestEngine_DiamondWithOnePathUnavailable(t *testing.T) {
	// Diamond: root → {left, right} → merge
	// left is unavailable, right completes. merge should be skipped (left dep unavailable).
	g := &Graph{
		Nodes: map[string]Node{
			"root":  newFuncNode("root", func(_ context.Context, _ *State) (any, error) { return "r", nil }),
			"left":  NewUnavailableNode("left"),
			"right": newFuncNode("right", func(_ context.Context, _ *State) (any, error) { return "r", nil }),
			"merge": newFuncNode("merge", func(_ context.Context, _ *State) (any, error) { return "m", nil }),
		},
		Edges: []Edge{
			{From: "root", To: "left"},
			{From: "root", To: "right"},
			{From: "left", To: "merge"},
			{From: "right", To: "merge"},
		},
		NodeDefs: map[string]NodeDef{
			"root":  {Component: "root"},
			"left":  {Component: "left", Optional: true},
			"right": {Component: "right"},
			"merge": {Component: "merge"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["root"].Status != StatusCompleted {
		t.Fatalf("expected root=%s, got %s", StatusCompleted, result.NodeResults["root"].Status)
	}
	if result.NodeResults["left"].Status != StatusUnavailable {
		t.Fatalf("expected left=%s, got %s", StatusUnavailable, result.NodeResults["left"].Status)
	}
	if result.NodeResults["right"].Status != StatusCompleted {
		t.Fatalf("expected right=%s, got %s", StatusCompleted, result.NodeResults["right"].Status)
	}
	// merge depends on left (unavailable) so it gets skipped
	if result.NodeResults["merge"].Status != StatusDepUnavailable {
		t.Fatalf("expected merge=%s, got %s", StatusDepUnavailable, result.NodeResults["merge"].Status)
	}
}

func TestEngine_DiamondWithContinueOnError(t *testing.T) {
	// Same diamond but left has on_error=continue → merge should run
	g := &Graph{
		Nodes: map[string]Node{
			"root":  newFuncNode("root", func(_ context.Context, _ *State) (any, error) { return "r", nil }),
			"left":  NewUnavailableNode("left"),
			"right": newFuncNode("right", func(_ context.Context, _ *State) (any, error) { return "r", nil }),
			"merge": newFuncNode("merge", func(_ context.Context, _ *State) (any, error) { return "m", nil }),
		},
		Edges: []Edge{
			{From: "root", To: "left"},
			{From: "root", To: "right"},
			{From: "left", To: "merge"},
			{From: "right", To: "merge"},
		},
		NodeDefs: map[string]NodeDef{
			"root":  {Component: "root"},
			"left":  {Component: "left", Optional: true, OnError: "continue"},
			"right": {Component: "right"},
			"merge": {Component: "merge"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["merge"].Status != StatusCompleted {
		t.Fatalf("expected merge=%s (left on_error=continue), got %s", StatusCompleted, result.NodeResults["merge"].Status)
	}
}

func TestEngine_StreamingCycleFreshStart(t *testing.T) {
	// Simulate: first cycle ser is unavailable, second cycle ser is available
	// This demonstrates that each cycle is independent — we swap the node.

	callCount := 0
	g := &Graph{
		Nodes: map[string]Node{
			"ser":        NewUnavailableNode("ser"),
			"compositor": newFuncNode("compositor", func(_ context.Context, _ *State) (any, error) { return "c", nil }),
		},
		Edges: []Edge{{From: "ser", To: "compositor"}},
		NodeDefs: map[string]NodeDef{
			"ser":        {Component: "ser", Optional: true},
			"compositor": {Component: "compositor"},
		},
	}

	engine := &Engine{}
	state := NewState()

	// Cycle 1: ser unavailable → compositor skipped
	result1, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("cycle 1 error: %v", err)
	}
	if result1.NodeResults["compositor"].Status != StatusDepUnavailable {
		t.Fatalf("cycle 1: expected compositor=%s, got %s", StatusDepUnavailable, result1.NodeResults["compositor"].Status)
	}

	// Simulate service becoming available: swap the node
	g.Nodes["ser"] = newFuncNode("ser", func(_ context.Context, _ *State) (any, error) {
		callCount++
		return "ser-output", nil
	})

	// Cycle 2: ser available → compositor runs
	result2, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("cycle 2 error: %v", err)
	}
	if result2.NodeResults["ser"].Status != StatusCompleted {
		t.Fatalf("cycle 2: expected ser=%s, got %s", StatusCompleted, result2.NodeResults["ser"].Status)
	}
	if result2.NodeResults["compositor"].Status != StatusCompleted {
		t.Fatalf("cycle 2: expected compositor=%s, got %s", StatusCompleted, result2.NodeResults["compositor"].Status)
	}
	if callCount != 1 {
		t.Fatalf("expected ser to run once in cycle 2, ran %d times", callCount)
	}
}

func TestEngine_NoNodeDefs_BackwardCompatible(t *testing.T) {
	// Old-style graph without NodeDefs — should work with default on_error=skip behavior
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, fmt.Errorf("fail")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		// NodeDefs is nil — backward compatibility
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("expected a=%s, got %s", StatusFailed, result.NodeResults["a"].Status)
	}
	// Default on_error=skip means b is skipped
	if result.NodeResults["b"].Status != StatusDepFailed {
		t.Fatalf("expected b=%s, got %s", StatusDepFailed, result.NodeResults["b"].Status)
	}
}

// =============================================================================
// NodeResult Helper Tests
// =============================================================================

func TestNodeResult_Helpers(t *testing.T) {
	tests := []struct {
		status     string
		isTerminal bool
		isSkipped  bool
		isSuccess  bool
	}{
		{StatusCompleted, true, false, true},
		{StatusFailed, true, false, false},
		{StatusSkipped, false, true, false},
		{StatusUnavailable, false, false, false},
		{StatusDepUnavailable, false, true, false},
		{StatusDepFailed, false, true, false},
	}

	for _, tt := range tests {
		nr := NodeResult{Status: tt.status}
		if nr.IsTerminal() != tt.isTerminal {
			t.Errorf("%s: IsTerminal=%v, want %v", tt.status, nr.IsTerminal(), tt.isTerminal)
		}
		if nr.IsSkipped() != tt.isSkipped {
			t.Errorf("%s: IsSkipped=%v, want %v", tt.status, nr.IsSkipped(), tt.isSkipped)
		}
		if nr.IsSuccess() != tt.isSuccess {
			t.Errorf("%s: IsSuccess=%v, want %v", tt.status, nr.IsSuccess(), tt.isSuccess)
		}
	}
}

// =============================================================================
// Full Pipeline Integration Test (simulates signal-analysis.yaml)
// =============================================================================

func TestEngine_SignalAnalysisPipeline_SERUnavailable(t *testing.T) {
	// Mimics the real signal-analysis.yaml:
	//   ser (optional) → signal_compositor → notes
	// When SER service is not available, all three are gracefully handled.

	reg := NewRegistry()
	// ser is NOT registered (service unavailable)
	reg.Register("signal_compositor", newFuncNode("signal_compositor", func(_ context.Context, _ *State) (any, error) {
		return "composed", nil
	}))
	reg.Register("notes", newFuncNode("notes", func(_ context.Context, _ *State) (any, error) {
		return "noted", nil
	}))

	p := &Pipeline{
		Name: "signal-analysis",
		Nodes: []NodeDef{
			{Component: "ser", Optional: true, Schedule: &ScheduleConfig{Interval: 3 * time.Second, MinBuffer: 1 * time.Second}},
			{Component: "signal_compositor", DependsOn: []string{"ser"}},
			{Component: "notes", DependsOn: []string{"signal_compositor"}},
		},
	}

	g, err := ResolvePipeline(p, reg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 nodes should be in the graph
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["ser"].Status != StatusUnavailable {
		t.Fatalf("expected ser=%s, got %s", StatusUnavailable, result.NodeResults["ser"].Status)
	}
	if result.NodeResults["signal_compositor"].Status != StatusDepUnavailable {
		t.Fatalf("expected signal_compositor=%s, got %s", StatusDepUnavailable, result.NodeResults["signal_compositor"].Status)
	}
	if result.NodeResults["notes"].Status != StatusDepUnavailable {
		t.Fatalf("expected notes=%s, got %s", StatusDepUnavailable, result.NodeResults["notes"].Status)
	}
}
