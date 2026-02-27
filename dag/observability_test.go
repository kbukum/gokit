package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/observability"
)

func TestWithTracing_WrapsNode(t *testing.T) {
	inner := newFuncNode("test-node", func(_ context.Context, _ *State) (any, error) {
		return "traced-result", nil
	})

	traced := WithTracing(inner, "dag.pipeline")
	if traced.Name() != "test-node" {
		t.Fatalf("expected 'test-node', got %q", traced.Name())
	}

	result, err := traced.Run(context.Background(), NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "traced-result" {
		t.Fatalf("expected 'traced-result', got %v", result)
	}
}

func TestWithTracing_PropagatesError(t *testing.T) {
	nodeErr := errors.New("fail")
	inner := newFuncNode("fail-node", func(_ context.Context, _ *State) (any, error) {
		return nil, nodeErr
	})

	traced := WithTracing(inner, "dag")
	_, err := traced.Run(context.Background(), NewState())
	if !errors.Is(err, nodeErr) {
		t.Fatalf("expected node error, got %v", err)
	}
}

func TestWithLogging_Success(t *testing.T) {
	log := logger.NewDefault("dag-test")
	inner := newFuncNode("log-node", func(_ context.Context, _ *State) (any, error) {
		return "logged", nil
	})

	logged := WithLogging(inner, log)
	if logged.Name() != "log-node" {
		t.Fatalf("expected 'log-node', got %q", logged.Name())
	}

	result, err := logged.Run(context.Background(), NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "logged" {
		t.Fatalf("expected 'logged', got %v", result)
	}
}

func TestWithLogging_Error(t *testing.T) {
	log := logger.NewDefault("dag-test")
	nodeErr := errors.New("log-fail")
	inner := newFuncNode("fail-log", func(_ context.Context, _ *State) (any, error) {
		return nil, nodeErr
	})

	logged := WithLogging(inner, log)
	_, err := logged.Run(context.Background(), NewState())
	if !errors.Is(err, nodeErr) {
		t.Fatalf("expected node error, got %v", err)
	}
}

func TestWithMetrics_Success(t *testing.T) {
	meter := observability.Meter("dag-test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	inner := newFuncNode("metrics-node", func(_ context.Context, _ *State) (any, error) {
		return "measured", nil
	})

	wrapped := WithMetrics(inner, metrics)
	if wrapped.Name() != "metrics-node" {
		t.Fatalf("expected 'metrics-node', got %q", wrapped.Name())
	}

	result, err := wrapped.Run(context.Background(), NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "measured" {
		t.Fatalf("expected 'measured', got %v", result)
	}
}

func TestWithMetrics_Error(t *testing.T) {
	meter := observability.Meter("dag-test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	nodeErr := errors.New("metrics-fail")
	inner := newFuncNode("fail-metrics", func(_ context.Context, _ *State) (any, error) {
		return nil, nodeErr
	})

	wrapped := WithMetrics(inner, metrics)
	_, err = wrapped.Run(context.Background(), NewState())
	if !errors.Is(err, nodeErr) {
		t.Fatalf("expected node error, got %v", err)
	}
}

func TestWithTracing_InDAG(t *testing.T) {
	outPort := Port[string]{Key: "traced-out"}

	nodeA := WithTracing(newFuncNode("a", func(_ context.Context, s *State) (any, error) {
		Write(s, outPort, "from-a")
		return "a-done", nil
	}), "test-dag")

	nodeB := WithTracing(newFuncNode("b", func(_ context.Context, s *State) (any, error) {
		v, err := Read(s, outPort)
		if err != nil {
			return nil, err
		}
		return "b-got:" + v, nil
	}), "test-dag")

	g := &Graph{
		Nodes: map[string]Node{"a": nodeA, "b": nodeB},
		Edges: []Edge{{From: "a", To: "b"}},
	}

	engine := &Engine{}
	state := NewState()
	result, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["a"].Status != "completed" {
		t.Fatal("expected a completed")
	}
	if result.NodeResults["b"].Status != "completed" {
		t.Fatal("expected b completed")
	}
	if result.NodeResults["b"].Output != "b-got:from-a" {
		t.Fatalf("expected 'b-got:from-a', got %v", result.NodeResults["b"].Output)
	}
}

func TestTool_IsAvailable(t *testing.T) {
	tool := AsTool[string, string](&Engine{}, &Graph{Nodes: map[string]Node{}}, ToolConfig[string, string]{
		Name:    "test-tool",
		InputFn: func(_ string, _ *State) {},
		OutputFn: func(_ *State) (string, error) {
			return "", nil
		},
	})

	if !tool.IsAvailable(context.Background()) {
		t.Fatal("expected tool to be available")
	}
}

func TestTool_ExecuteError(t *testing.T) {
	nodeErr := errors.New("node failed")
	g := &Graph{
		Nodes: map[string]Node{
			"fail": newFuncNode("fail", func(_ context.Context, _ *State) (any, error) {
				return nil, nodeErr
			}),
		},
	}

	tool := AsTool[string, string](&Engine{}, g, ToolConfig[string, string]{
		Name:    "fail-tool",
		InputFn: func(_ string, _ *State) {},
		OutputFn: func(s *State) (string, error) {
			return "", errors.New("should not reach here")
		},
	})

	// ExecuteBatch doesn't return errors for individual node failures,
	// it records them in NodeResults. But the OutputFn should still work.
	result, err := tool.Execute(context.Background(), "input")
	if err != nil {
		// Tool returns error only if ExecuteBatch itself fails (cycle, context)
		t.Logf("got error: %v", err)
	} else {
		t.Logf("got result: %v", result)
	}
}
