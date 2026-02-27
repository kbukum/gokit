package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/dag"
)

func TestMockNode_Basic(t *testing.T) {
	node := NewMockNode("test", "hello", nil)
	result, err := node.Run(context.Background(), dag.NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %v", result)
	}
	if node.Calls() != 1 {
		t.Fatalf("expected 1 call, got %d", node.Calls())
	}
}

func TestMockNode_Reset(t *testing.T) {
	node := NewMockNode("test", nil, nil)
	node.Run(context.Background(), dag.NewState())
	node.Run(context.Background(), dag.NewState())
	if node.Calls() != 2 {
		t.Fatalf("expected 2 calls, got %d", node.Calls())
	}
	node.Reset()
	if node.Calls() != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", node.Calls())
	}
}

func TestMockNodeFunc(t *testing.T) {
	port := dag.Port[string]{Key: "out"}
	node := NewMockNodeFunc("writer", func(_ context.Context, s *dag.State) (any, error) {
		dag.Write(s, port, "written")
		return "written", nil
	})

	state := dag.NewState()
	_, err := node.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, err := dag.Read(state, port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "written" {
		t.Fatalf("expected 'written', got %q", val)
	}
}

func TestGraphBuilder(t *testing.T) {
	g := NewGraphBuilder().
		AddNode(NewMockNode("a", "result-a", nil)).
		AddNode(NewMockNode("b", "result-b", nil)).
		AddEdge("a", "b").
		Build()

	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}

	engine := &dag.Engine{}
	result, err := engine.ExecuteBatch(context.Background(), g, dag.NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NodeResults["a"].Status != "completed" {
		t.Fatal("expected a completed")
	}
	if result.NodeResults["b"].Status != "completed" {
		t.Fatal("expected b completed")
	}
}

func TestComponent_Lifecycle(t *testing.T) {
	g := NewGraphBuilder().
		AddNode(NewMockNode("step", "done", nil)).
		Build()

	comp := NewComponent(g)

	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	result, err := comp.Execute(context.Background())
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.NodeResults["step"].Status != "completed" {
		t.Fatal("expected step completed")
	}

	if err := comp.Reset(context.Background()); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	mockNode := g.Nodes["step"].(*MockNode)
	if mockNode.Calls() != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", mockNode.Calls())
	}

	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}
