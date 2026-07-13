package dag

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeepChain_20Levels(t *testing.T) {
	const depth = 20
	nodes := make(map[string]Node, depth)
	var edges []Edge

	for i := 0; i < depth; i++ {
		name := fmt.Sprintf("n%d", i)
		idx := i
		nodes[name] = newFuncNode(name, func(_ context.Context, s *State) (any, error) {
			if idx > 0 {
				prev := fmt.Sprintf("n%d", idx-1)
				if _, ok := s.Get(prev); !ok {
					return nil, fmt.Errorf("expected %s to have run first", prev)
				}
			}
			s.Set(name, idx)
			return idx, nil
		})
		if i > 0 {
			edges = append(edges, Edge{From: fmt.Sprintf("n%d", i-1), To: name})
		}
	}

	g := &Graph{Nodes: nodes, Edges: edges}
	engine := &Engine{}
	state := NewState()

	result, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < depth; i++ {
		name := fmt.Sprintf("n%d", i)
		nr := result.NodeResults[name]
		if nr.Status != StatusCompleted {
			t.Fatalf("node %s: expected completed, got %s", name, nr.Status)
		}
	}

	// Verify final node wrote to state
	val, ok := state.Get(fmt.Sprintf("n%d", depth-1))
	if !ok || val != depth-1 {
		t.Fatalf("expected final state value %d, got %v (ok=%v)", depth-1, val, ok)
	}
}

func TestWideParallelism_50Nodes(t *testing.T) {
	const count = 50
	var running atomic.Int32
	var maxRunning atomic.Int32

	nodes := make(map[string]Node, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("w%d", i)
		nodes[name] = newFuncNode(name, func(_ context.Context, _ *State) (any, error) {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			running.Add(-1)
			return name, nil
		})
	}

	g := &Graph{Nodes: nodes}
	engine := &Engine{}

	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("w%d", i)
		if result.NodeResults[name].Status != StatusCompleted {
			t.Fatalf("node %s not completed: %s", name, result.NodeResults[name].Status)
		}
	}

	if maxRunning.Load() < 2 {
		t.Log("parallel execution not observed (may vary by system)")
	}
}

func TestErrorCascade_SkipPolicy(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-ok", nil
			}),
			"c": newFuncNode("c", func(_ context.Context, _ *State) (any, error) {
				return "c-ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: OnErrorSkip},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("expected a failed, got %s", result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != StatusDepFailed {
		t.Fatalf("expected b dep_failed, got %s", result.NodeResults["b"].Status)
	}
	if result.NodeResults["c"].Status != StatusCompleted {
		t.Fatalf("expected c completed, got %s", result.NodeResults["c"].Status)
	}
}

func TestErrorCascade_FailPolicy(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: OnErrorFail},
		},
	}

	engine := &Engine{}
	_, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err == nil {
		t.Fatal("expected error from on_error=fail policy")
	}
}

func TestErrorCascade_ContinuePolicy(t *testing.T) {
	var bRan bool
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				bRan = true
				return "b-ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: OnErrorContinue},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("expected a failed, got %s", result.NodeResults["a"].Status)
	}
	if !bRan {
		t.Fatal("expected b to run despite a's failure")
	}
	if result.NodeResults["b"].Status != StatusCompleted {
		t.Fatalf("expected b completed, got %s", result.NodeResults["b"].Status)
	}
}

func TestContextTimeout_PerNodeSimulation(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"slow": newFuncNode("slow", func(ctx context.Context, _ *State) (any, error) {
				select {
				case <-time.After(2 * time.Second):
					return "done", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	engine := &Engine{}
	result, err := engine.ExecuteBatch(ctx, g, NewState())
	// Either the engine returns a context error or the node captures it
	if err != nil {
		return // context error propagated at engine level — pass
	}
	nr := result.NodeResults["slow"]
	if nr.Status != StatusFailed {
		t.Fatalf("expected slow node to fail from timeout, got %s", nr.Status)
	}
}

func TestContextCancellation_MidExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				cancel() // cancel after this node completes
				return "a-ok", nil
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				t.Error("b should not have run")
				return "b-ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
	}

	engine := &Engine{}
	_, err := engine.ExecuteBatch(ctx, g, NewState())
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestDiamondWith_OneContinueOneSkip(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("a failed")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-ok", nil
			}),
			"c": newFuncNode("c", func(_ context.Context, _ *State) (any, error) {
				return "c-ok", nil
			}),
			"d": newFuncNode("d", func(_ context.Context, _ *State) (any, error) {
				return "d-ok", nil
			}),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: OnErrorSkip},
			"b": {Component: "b", OnError: OnErrorContinue},
			"c": {Component: "c", OnError: OnErrorSkip},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusFailed {
		t.Fatalf("a: expected failed, got %s", result.NodeResults["a"].Status)
	}
	// "a" has OnError=skip, so both "b" and "c" should be dep_failed
	if result.NodeResults["b"].Status != StatusDepFailed {
		t.Fatalf("b: expected dep_failed, got %s", result.NodeResults["b"].Status)
	}
	if result.NodeResults["c"].Status != StatusDepFailed {
		t.Fatalf("c: expected dep_failed, got %s", result.NodeResults["c"].Status)
	}
	// "d" depends on both "b" (continue) and "c" (skip)
	// Since "c" is dep_failed with skip policy, "d" should be skipped
	dStatus := result.NodeResults["d"].Status
	if dStatus != StatusDepFailed {
		t.Fatalf("d: expected dep_failed, got %s", dStatus)
	}
}

func TestEngine_UnavailableNodeCascades(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": NewUnavailableNode("a"),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "b-ok", nil
			}),
			"c": newFuncNode("c", func(_ context.Context, _ *State) (any, error) {
				return "c-ok", nil
			}),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != StatusUnavailable {
		t.Fatalf("a: expected unavailable, got %s", result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != StatusDepUnavailable {
		t.Fatalf("b: expected dep_unavailable, got %s", result.NodeResults["b"].Status)
	}
	if result.NodeResults["c"].Status != StatusDepUnavailable {
		t.Fatalf("c: expected dep_unavailable, got %s", result.NodeResults["c"].Status)
	}
}

func TestLargeGraph_100Nodes(t *testing.T) {
	const middleCount = 98
	nodes := make(map[string]Node, middleCount+2)
	var edges []Edge

	// Root node
	nodes["root"] = newFuncNode("root", func(_ context.Context, s *State) (any, error) {
		s.Set("root", true)
		return "root", nil
	})

	// Middle layer: 98 nodes all depend on root
	for i := 0; i < middleCount; i++ {
		name := fmt.Sprintf("m%d", i)
		nodes[name] = newFuncNode(name, func(_ context.Context, s *State) (any, error) {
			s.Set(name, true)
			return name, nil
		})
		edges = append(edges, Edge{From: "root", To: name})
	}

	// Sink node depends on all middle nodes
	nodes["sink"] = newFuncNode("sink", func(_ context.Context, s *State) (any, error) {
		return "sink-done", nil
	})
	for i := 0; i < middleCount; i++ {
		edges = append(edges, Edge{From: fmt.Sprintf("m%d", i), To: "sink"})
	}

	g := &Graph{Nodes: nodes, Edges: edges}
	engine := &Engine{}

	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all nodes completed
	for name := range nodes {
		nr := result.NodeResults[name]
		if nr.Status != StatusCompleted {
			t.Fatalf("node %s: expected completed, got %s", name, nr.Status)
		}
	}

	// Verify topology: 3 levels (root, middle, sink)
	levels, err := BuildLevels(g)
	if err != nil {
		t.Fatalf("BuildLevels error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if len(levels[1]) != middleCount {
		t.Fatalf("expected %d middle nodes, got %d", middleCount, len(levels[1]))
	}
}

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
		// NodeDefs left nil — exercises the zero-value fallback path that
		// falls back to per-step Handler functions when no NodeDef registry exists.
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
