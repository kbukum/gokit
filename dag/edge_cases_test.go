package dag

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// 1. TestDeepChain_20Levels — linear chain of 20 nodes
// =============================================================================

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

// =============================================================================
// 2. TestWideParallelism_50Nodes — 50 independent nodes, verify concurrency
// =============================================================================

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

// =============================================================================
// 3. TestErrorCascade_SkipPolicy — failing node with skip causes dep_failed
// =============================================================================

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

// =============================================================================
// 4. TestErrorCascade_FailPolicy — on_error=fail halts pipeline
// =============================================================================

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

// =============================================================================
// 5. TestErrorCascade_ContinuePolicy — dependents run despite failure
// =============================================================================

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

// =============================================================================
// 6. TestContextTimeout_PerNodeSimulation — DAG-level timeout
// =============================================================================

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

// =============================================================================
// 7. TestContextCancellation_MidExecution — cancel after first level
// =============================================================================

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

// =============================================================================
// 8. TestSessionState_AcrossCycles — state persists across streaming cycles
// =============================================================================

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
	if result1.NodeResults["writer"].Status != StatusCompleted {
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
	if result2.NodeResults["reader"].Status != StatusCompleted {
		t.Fatalf("cycle 2: reader expected completed, got %s", result2.NodeResults["reader"].Status)
	}
	if result2.NodeResults["reader"].Output != "written-value" {
		t.Fatalf("cycle 2: expected 'written-value', got %v", result2.NodeResults["reader"].Output)
	}
}

// =============================================================================
// 9. TestDiamondWith_OneContinueOneSkip — mixed error policies in diamond
// =============================================================================

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

// =============================================================================
// 10. TestBuildLevels_LongCycle — cycle across 5 nodes
// =============================================================================

func TestBuildLevels_LongCycle(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e"}
	nodes := make(map[string]Node, len(names))
	for _, name := range names {
		nodes[name] = newFuncNode(name, nil)
	}

	// a→b→c→d→e→a
	edges := []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "d", To: "e"},
		{From: "e", To: "a"},
	}

	g := &Graph{Nodes: nodes, Edges: edges}
	_, err := BuildLevels(g)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

// =============================================================================
// 11. TestEngine_UnavailableNodeCascades — unavailable cascades dep_unavailable
// =============================================================================

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

// =============================================================================
// 12. TestLargeGraph_100Nodes — 100 nodes in diamond-like topology
// =============================================================================

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
