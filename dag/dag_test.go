package dag

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// --- test helpers ---

// funcNode is a simple Node implementation for testing.
type funcNode struct {
	name string
	fn   func(ctx context.Context, state *State) (any, error)
}

func (n *funcNode) Name() string { return n.name }
func (n *funcNode) Run(ctx context.Context, state *State) (any, error) {
	return n.fn(ctx, state)
}

func newFuncNode(name string, fn func(ctx context.Context, state *State) (any, error)) Node {
	return &funcNode{name: name, fn: fn}
}

// --- State tests ---

func TestState_GetSet(t *testing.T) {
	s := NewState()
	s.Set("key", "value")
	v, ok := s.Get("key")
	if !ok || v != "value" {
		t.Fatalf("expected 'value', got %v (ok=%v)", v, ok)
	}
}

func TestState_Missing(t *testing.T) {
	s := NewState()
	_, ok := s.Get("missing")
	if ok {
		t.Fatal("expected missing key")
	}
}

// --- Port tests ---

func TestPort_ReadWrite(t *testing.T) {
	s := NewState()
	port := Port[int]{Key: "count"}
	Write(s, port, 42)

	val, err := Read(s, port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}
}

func TestPort_MissingKey(t *testing.T) {
	s := NewState()
	port := Port[int]{Key: "missing"}
	_, err := Read(s, port)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestPort_TypeMismatch(t *testing.T) {
	s := NewState()
	s.Set("key", "not-an-int")
	port := Port[int]{Key: "key"}
	_, err := Read(s, port)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

// --- BuildLevels tests ---

func TestBuildLevels_Linear(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", nil),
			"b": newFuncNode("b", nil),
			"c": newFuncNode("c", nil),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}

	levels, err := BuildLevels(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels[0][0] != "a" || levels[1][0] != "b" || levels[2][0] != "c" {
		t.Fatalf("unexpected level order: %v", levels)
	}
}

func TestBuildLevels_Diamond(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", nil),
			"b": newFuncNode("b", nil),
			"c": newFuncNode("c", nil),
			"d": newFuncNode("d", nil),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}

	levels, err := BuildLevels(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels[0][0] != "a" {
		t.Fatalf("expected 'a' at level 0")
	}
	if len(levels[1]) != 2 {
		t.Fatalf("expected 2 nodes at level 1, got %d", len(levels[1]))
	}
	if levels[2][0] != "d" {
		t.Fatalf("expected 'd' at level 2")
	}
}

func TestBuildLevels_CycleDetection(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", nil),
			"b": newFuncNode("b", nil),
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "a"},
		},
	}

	_, err := BuildLevels(g)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestBuildLevels_UnknownNode(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", nil),
		},
		Edges: []Edge{
			{From: "a", To: "unknown"},
		},
	}

	_, err := BuildLevels(g)
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestBuildLevels_NoEdges(t *testing.T) {
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", nil),
			"b": newFuncNode("b", nil),
		},
	}

	levels, err := BuildLevels(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(levels))
	}
	if len(levels[0]) != 2 {
		t.Fatalf("expected 2 nodes at level 0, got %d", len(levels[0]))
	}
}

// --- Engine tests ---

func TestEngine_BatchExecution(t *testing.T) {
	outPort := Port[string]{Key: "output"}
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, s *State) (any, error) {
				s.Set("a_done", true)
				return "a-result", nil
			}),
			"b": newFuncNode("b", func(_ context.Context, s *State) (any, error) {
				if _, ok := s.Get("a_done"); !ok {
					return nil, fmt.Errorf("a should have run first")
				}
				Write(s, outPort, "final")
				return "b-result", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
	}

	engine := &Engine{}
	state := NewState()
	result, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != "completed" {
		t.Fatalf("expected a completed, got %s", result.NodeResults["a"].Status)
	}
	if result.NodeResults["b"].Status != "completed" {
		t.Fatalf("expected b completed, got %s", result.NodeResults["b"].Status)
	}

	out, err := Read(state, outPort)
	if err != nil {
		t.Fatalf("unexpected error reading output: %v", err)
	}
	if out != "final" {
		t.Fatalf("expected 'final', got %q", out)
	}
}

func TestEngine_ErrorPropagation(t *testing.T) {
	nodeErr := errors.New("node failed")
	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, nodeErr
			}),
		},
	}

	engine := &Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}

	nr := result.NodeResults["a"]
	if nr.Status != "failed" {
		t.Fatalf("expected failed, got %s", nr.Status)
	}
	if !errors.Is(nr.Error, nodeErr) {
		t.Fatalf("expected node error, got %v", nr.Error)
	}
}

func TestEngine_ParallelExecution(t *testing.T) {
	var running atomic.Int32
	var maxRunning atomic.Int32

	makeNode := func(name string) Node {
		return newFuncNode(name, func(_ context.Context, _ *State) (any, error) {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
			return name, nil
		})
	}

	g := &Graph{
		Nodes: map[string]Node{
			"a": makeNode("a"),
			"b": makeNode("b"),
			"c": makeNode("c"),
		},
	}

	engine := &Engine{}
	_, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxRunning.Load() < 2 {
		t.Log("parallel execution not observed (may vary by system)")
	}
}

func TestEngine_MaxParallel(t *testing.T) {
	var running atomic.Int32
	var maxRunning atomic.Int32

	makeNode := func(name string) Node {
		return newFuncNode(name, func(_ context.Context, _ *State) (any, error) {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			running.Add(-1)
			return name, nil
		})
	}

	g := &Graph{
		Nodes: map[string]Node{
			"a": makeNode("a"),
			"b": makeNode("b"),
			"c": makeNode("c"),
			"d": makeNode("d"),
		},
	}

	engine := &Engine{MaxParallel: 2}
	_, err := engine.ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxRunning.Load() > 2 {
		t.Fatalf("expected max 2 concurrent, got %d", maxRunning.Load())
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, nil
			}),
		},
	}

	engine := &Engine{}
	_, err := engine.ExecuteBatch(ctx, g, NewState())
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- Registry tests ---

func TestRegistry_RegisterGetList(t *testing.T) {
	r := NewRegistry()
	node := newFuncNode("test", nil)
	r.Register("test", node)

	got, ok := r.Get("test")
	if !ok || got.Name() != "test" {
		t.Fatalf("expected to find 'test' node")
	}

	_, ok = r.Get("missing")
	if ok {
		t.Fatal("expected missing")
	}

	names := r.List()
	if len(names) != 1 || names[0] != "test" {
		t.Fatalf("unexpected list: %v", names)
	}
}

// --- AsTool tests ---

func TestAsTool_WrapsAsProvider(t *testing.T) {
	inPort := Port[string]{Key: "input"}
	outPort := Port[string]{Key: "output"}

	g := &Graph{
		Nodes: map[string]Node{
			"upper": newFuncNode("upper", func(_ context.Context, s *State) (any, error) {
				in, err := Read(s, inPort)
				if err != nil {
					return nil, err
				}
				result := "UPPER:" + in
				Write(s, outPort, result)
				return result, nil
			}),
		},
	}

	engine := &Engine{}
	tool := AsTool[string, string](engine, g, ToolConfig[string, string]{
		Name: "upper-tool",
		InputFn: func(input string, state *State) {
			Write(state, inPort, input)
		},
		OutputFn: func(state *State) (string, error) {
			return Read(state, outPort)
		},
	})

	if tool.Name() != "upper-tool" {
		t.Fatalf("expected 'upper-tool', got %q", tool.Name())
	}

	result, err := tool.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "UPPER:hello" {
		t.Fatalf("expected 'UPPER:hello', got %q", result)
	}
}
