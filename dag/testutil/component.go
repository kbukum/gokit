package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/dag"
	"github.com/kbukum/gokit/testutil"
)

// MockNode is a configurable test node for DAG testing.
// It records calls and returns a preset output or error.
type MockNode struct {
	name   string
	output any
	err    error
	fn     func(ctx context.Context, state *dag.State) (any, error)

	mu    sync.Mutex
	calls int
}

var _ dag.Node = (*MockNode)(nil)

// NewMockNode creates a mock node that returns the given output.
// If err is non-nil, the node will fail with that error.
func NewMockNode(name string, output any, err error) *MockNode {
	return &MockNode{name: name, output: output, err: err}
}

// NewMockNodeFunc creates a mock node backed by a custom function.
func NewMockNodeFunc(name string, fn func(ctx context.Context, state *dag.State) (any, error)) *MockNode {
	return &MockNode{name: name, fn: fn}
}

func (n *MockNode) Name() string { return n.name }

func (n *MockNode) Run(ctx context.Context, state *dag.State) (any, error) {
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()

	if n.fn != nil {
		return n.fn(ctx, state)
	}
	return n.output, n.err
}

// Calls returns how many times Run was invoked.
func (n *MockNode) Calls() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls
}

// Reset clears the call counter.
func (n *MockNode) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = 0
}

// GraphBuilder provides a fluent API for constructing test graphs.
type GraphBuilder struct {
	nodes map[string]dag.Node
	edges []dag.Edge
}

// NewGraphBuilder creates a new GraphBuilder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{nodes: make(map[string]dag.Node)}
}

// AddNode adds a node to the graph.
func (b *GraphBuilder) AddNode(node dag.Node) *GraphBuilder {
	b.nodes[node.Name()] = node
	return b
}

// AddEdge adds a dependency edge (from depends on to).
func (b *GraphBuilder) AddEdge(from, to string) *GraphBuilder {
	b.edges = append(b.edges, dag.Edge{From: from, To: to})
	return b
}

// Build returns the constructed Graph.
func (b *GraphBuilder) Build() *dag.Graph {
	return &dag.Graph{Nodes: b.nodes, Edges: b.edges}
}

// Component is a test DAG component that implements testutil.TestComponent.
// It wraps a graph + engine for integration testing with gokit's test lifecycle.
type Component struct {
	engine *dag.Engine
	graph  *dag.Graph
	state  *dag.State
	result *dag.Result

	mu      sync.RWMutex
	started bool
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new test DAG component.
func NewComponent(graph *dag.Graph) *Component {
	return &Component{
		engine: &dag.Engine{},
		graph:  graph,
		state:  dag.NewState(),
	}
}

// WithMaxParallel sets the engine's max parallelism.
func (c *Component) WithMaxParallel(n int) *Component {
	c.engine.MaxParallel = n
	return c
}

// State returns the current execution state.
func (c *Component) State() *dag.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// LastResult returns the result from the most recent execution.
func (c *Component) LastResult() *dag.Result {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result
}

// Execute runs the graph and stores the result.
func (c *Component) Execute(ctx context.Context) (*dag.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.engine.ExecuteBatch(ctx, c.graph, c.state)
	if err == nil {
		c.result = result
	}
	return result, err
}

// --- component.Component ---

func (c *Component) Name() string { return "dag-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = dag.NewState()
	c.result = nil
	// Reset mock nodes
	for _, node := range c.graph.Nodes {
		if mock, ok := node.(*MockNode); ok {
			mock.Reset()
		}
	}
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result, nil
}

func (c *Component) Restore(_ context.Context, _ interface{}) error {
	return nil
}
