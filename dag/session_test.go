package dag

import (
	"context"
	"testing"
	"time"
)

func TestSession_ReadyFilter_NoSchedule(t *testing.T) {
	sess := NewSession("test")
	pipeline := &Pipeline{
		Nodes: []NodeDef{
			{Component: "always-run"},
		},
	}

	filter := sess.ReadyFilter(pipeline, nil)
	if !filter("always-run", sess.State) {
		t.Fatal("expected node with no schedule to be ready")
	}
}

func TestSession_ReadyFilter_Interval(t *testing.T) {
	sess := NewSession("test")
	pipeline := &Pipeline{
		Nodes: []NodeDef{
			{Component: "periodic", Schedule: &ScheduleConfig{Interval: 100 * time.Millisecond}},
		},
	}

	filter := sess.ReadyFilter(pipeline, nil)

	// First call should be ready
	if !filter("periodic", sess.State) {
		t.Fatal("expected first call to be ready")
	}

	// Immediate second call should not be ready
	if filter("periodic", sess.State) {
		t.Fatal("expected immediate re-call to be skipped")
	}

	// After interval, should be ready again
	time.Sleep(110 * time.Millisecond)
	if !filter("periodic", sess.State) {
		t.Fatal("expected call after interval to be ready")
	}
}

func TestSession_ReadyFilter_MinBuffer(t *testing.T) {
	sess := NewSession("test")
	pipeline := &Pipeline{
		Nodes: []NodeDef{
			{Component: "buffered", Schedule: &ScheduleConfig{MinBuffer: 50 * time.Millisecond}},
		},
	}

	filter := sess.ReadyFilter(pipeline, nil)

	// Should not be ready before min_buffer
	if filter("buffered", sess.State) {
		t.Fatal("expected not ready before min_buffer")
	}

	time.Sleep(60 * time.Millisecond)
	if !filter("buffered", sess.State) {
		t.Fatal("expected ready after min_buffer")
	}
}

func TestSession_ReadyFilter_Condition(t *testing.T) {
	sess := NewSession("test")
	pipeline := &Pipeline{
		Nodes: []NodeDef{
			{Component: "conditional", Condition: "has-data"},
		},
	}

	conditions := map[string]ConditionFunc{
		"has-data": func(state *State) bool {
			_, ok := state.Get("data")
			return ok
		},
	}

	filter := sess.ReadyFilter(pipeline, conditions)

	// No data -> not ready
	if filter("conditional", sess.State) {
		t.Fatal("expected not ready without data")
	}

	// Set data -> ready
	sess.State.Set("data", "value")
	if !filter("conditional", sess.State) {
		t.Fatal("expected ready with data")
	}
}

func TestEngine_ExecuteStreaming(t *testing.T) {
	callCount := make(map[string]int)
	makeNode := func(name string) Node {
		return newFuncNode(name, func(_ context.Context, _ *State) (any, error) {
			callCount[name]++
			return name, nil
		})
	}

	g := &Graph{
		Nodes: map[string]Node{
			"a": makeNode("a"),
			"b": makeNode("b"),
			"c": makeNode("c"),
		},
		Edges: []Edge{
			{From: "a", To: "c"},
			{From: "b", To: "c"},
		},
	}

	engine := &Engine{}
	state := NewState()

	// Only run "a" and "c"
	filter := func(name string, _ *State) bool {
		return name == "a" || name == "c"
	}

	result, err := engine.ExecuteStreaming(context.Background(), g, state, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != "completed" {
		t.Fatalf("expected a completed, got %s", result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != "skipped" {
		t.Fatalf("expected b skipped, got %s", result.NodeResults["b"].Status)
	}
	if result.NodeResults["c"].Status != "completed" {
		t.Fatalf("expected c completed, got %s", result.NodeResults["c"].Status)
	}
}
