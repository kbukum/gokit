package dag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/dag/status"
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
		return newFuncNode(name, func(_ context.Context, s *State) (any, error) {
			callCount[name]++
			s.Set(name, name) // Write output to state so dependents can access it
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

	// Cycle 1: only run "a" — "b" is skipped with no state, so "c" should also be skipped
	filter := func(name string, _ *State) bool {
		return name == "a" || name == "c"
	}

	result, err := engine.ExecuteStreaming(context.Background(), g, state, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != status.Completed {
		t.Fatalf("expected a completed, got %s", result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != status.Skipped {
		t.Fatalf("expected b skipped, got %s", result.NodeResults["b"].Status)
	}
	// "c" should be dep-skipped because "b" was skipped and has no state output
	if result.NodeResults["c"].Status != status.DepSkipped {
		t.Fatalf("expected c dep-skipped, got %s", result.NodeResults["c"].Status)
	}

	// Cycle 2: run "b" to populate state
	filter2 := func(name string, _ *State) bool {
		return name == "b"
	}
	result2, err := engine.ExecuteStreaming(context.Background(), g, state, filter2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.NodeResults["b"].Status != status.Completed {
		t.Fatalf("expected b completed, got %s", result2.NodeResults["b"].Status)
	}

	// Cycle 3: "b" is skipped again, but state has "b" from cycle 2, so "c" should run
	result3, err := engine.ExecuteStreaming(context.Background(), g, state, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result3.NodeResults["c"].Status != status.Completed {
		t.Fatalf("expected c completed with cached state, got %s", result3.NodeResults["c"].Status)
	}
}

func TestSessionState_AcrossCycles(t *testing.T) {
	sess := NewSession("persist-test")
	pipeline := &Pipeline{
		Nodes: []NodeDef{
			{Component: "writer"},
			{Component: "reader", DependsOn: []string{"writer"}},
		},
	}

	g := &Graph{
		Nodes: map[string]Node{
			"writer": newFuncNode("writer", func(_ context.Context, s *State) (any, error) {
				s.Set("writer", "written-value")
				return "wrote", nil
			}),
			"reader": newFuncNode("reader", func(_ context.Context, s *State) (any, error) {
				val, ok := s.Get("writer")
				if !ok {
					return nil, errors.New("writer state not found")
				}
				return val, nil
			}),
		},
		Edges: []Edge{{From: "writer", To: "reader"}},
	}

	engine := &Engine{}

	// Cycle 1: run only "writer"
	filter1 := sess.ReadyFilter(pipeline, nil)
	// Override to only run writer in cycle 1
	cycle1Filter := func(name string, s *State) bool {
		if name == "reader" {
			return false
		}
		return filter1(name, s)
	}

	result1, err := engine.ExecuteStreaming(context.Background(), g, sess.State, cycle1Filter)
	if err != nil {
		t.Fatalf("cycle 1 error: %v", err)
	}
	if result1.NodeResults["writer"].Status != status.Completed {
		t.Fatalf("cycle 1: writer expected completed, got %s", result1.NodeResults["writer"].Status)
	}

	// Cycle 2: run "reader" — should see writer's state from cycle 1
	cycle2Filter := func(name string, s *State) bool {
		return name == "reader"
	}

	result2, err := engine.ExecuteStreaming(context.Background(), g, sess.State, cycle2Filter)
	if err != nil {
		t.Fatalf("cycle 2 error: %v", err)
	}
	if result2.NodeResults["reader"].Status != status.Completed {
		t.Fatalf("cycle 2: reader expected completed, got %s", result2.NodeResults["reader"].Status)
	}
	if result2.NodeResults["reader"].Output != "written-value" {
		t.Fatalf("cycle 2: expected 'written-value', got %v", result2.NodeResults["reader"].Output)
	}
}
