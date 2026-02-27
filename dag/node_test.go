package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/provider"
)

// stubProvider implements provider.RequestResponse for testing.
type stubProvider struct {
	name   string
	execFn func(ctx context.Context, input string) (string, error)
}

func (s *stubProvider) Name() string                       { return s.name }
func (s *stubProvider) IsAvailable(_ context.Context) bool { return true }
func (s *stubProvider) Execute(ctx context.Context, input string) (string, error) {
	return s.execFn(ctx, input)
}

var _ provider.RequestResponse[string, string] = (*stubProvider)(nil)

func TestFromProvider_BasicExecution(t *testing.T) {
	inputPort := Port[string]{Key: "input"}
	outputPort := Port[string]{Key: "output"}

	svc := &stubProvider{
		name: "upper",
		execFn: func(_ context.Context, input string) (string, error) {
			return "UPPER:" + input, nil
		},
	}

	node := FromProvider(NodeConfig[string, string]{
		Name:    "upper-node",
		Service: svc,
		Extract: func(state *State) (string, error) {
			return Read(state, inputPort)
		},
		Output: outputPort,
	})

	if node.Name() != "upper-node" {
		t.Fatalf("expected 'upper-node', got %q", node.Name())
	}

	state := NewState()
	Write(state, inputPort, "hello")

	result, err := node.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "UPPER:hello" {
		t.Fatalf("expected 'UPPER:hello', got %v", result)
	}

	out, err := Read(state, outputPort)
	if err != nil {
		t.Fatalf("unexpected error reading output: %v", err)
	}
	if out != "UPPER:hello" {
		t.Fatalf("expected 'UPPER:hello' in state, got %q", out)
	}
}

func TestFromProvider_ExtractError(t *testing.T) {
	extractErr := errors.New("bad extract")
	node := FromProvider(NodeConfig[string, string]{
		Name:    "fail-extract",
		Service: &stubProvider{name: "svc", execFn: nil},
		Extract: func(_ *State) (string, error) {
			return "", extractErr
		},
		Output: Port[string]{Key: "out"},
	})

	_, err := node.Run(context.Background(), NewState())
	if !errors.Is(err, extractErr) {
		t.Fatalf("expected extract error, got %v", err)
	}
}

func TestFromProvider_ServiceError(t *testing.T) {
	svcErr := errors.New("service failed")
	node := FromProvider(NodeConfig[string, string]{
		Name: "fail-svc",
		Service: &stubProvider{
			name: "svc",
			execFn: func(_ context.Context, _ string) (string, error) {
				return "", svcErr
			},
		},
		Extract: func(_ *State) (string, error) {
			return "input", nil
		},
		Output: Port[string]{Key: "out"},
	})

	_, err := node.Run(context.Background(), NewState())
	if !errors.Is(err, svcErr) {
		t.Fatalf("expected service error, got %v", err)
	}
}

func TestFromProvider_InDAG(t *testing.T) {
	rawPort := Port[string]{Key: "raw"}
	resultPort := Port[string]{Key: "result"}

	extractNode := FromProvider(NodeConfig[string, string]{
		Name: "extract",
		Service: &stubProvider{
			name: "extractor",
			execFn: func(_ context.Context, _ string) (string, error) {
				return "raw-data", nil
			},
		},
		Extract: func(_ *State) (string, error) { return "", nil },
		Output:  rawPort,
	})

	transformNode := FromProvider(NodeConfig[string, string]{
		Name: "transform",
		Service: &stubProvider{
			name: "transformer",
			execFn: func(_ context.Context, input string) (string, error) {
				return "processed:" + input, nil
			},
		},
		Extract: func(state *State) (string, error) {
			return Read(state, rawPort)
		},
		Output: resultPort,
	})

	g := &Graph{
		Nodes: map[string]Node{
			"extract":   extractNode,
			"transform": transformNode,
		},
		Edges: []Edge{{From: "extract", To: "transform"}},
	}

	engine := &Engine{}
	state := NewState()
	result, err := engine.ExecuteBatch(context.Background(), g, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.NodeResults["extract"].Status != "completed" {
		t.Fatal("expected extract completed")
	}
	if result.NodeResults["transform"].Status != "completed" {
		t.Fatal("expected transform completed")
	}

	out, err := Read(state, resultPort)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "processed:raw-data" {
		t.Fatalf("expected 'processed:raw-data', got %q", out)
	}
}
