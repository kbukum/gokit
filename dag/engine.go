package dag

import (
	"context"
	"sync"
	"time"
)

// Engine executes a graph in dependency order.
type Engine struct {
	// MaxParallel limits concurrent nodes per level (0 = unlimited).
	MaxParallel int
}

// ExecuteBatch runs ALL nodes in dependency order, one-shot.
func (e *Engine) ExecuteBatch(ctx context.Context, g *Graph, state *State) (*Result, error) {
	return e.execute(ctx, g, state, nil)
}

// ExecuteStreaming runs only nodes that pass the filter.
// Nodes that don't pass are marked as "skipped".
func (e *Engine) ExecuteStreaming(ctx context.Context, g *Graph, state *State, filter NodeFilter) (*Result, error) {
	return e.execute(ctx, g, state, filter)
}

// NodeFilter returns true if a node should execute in this cycle.
type NodeFilter func(nodeName string, state *State) bool

func (e *Engine) execute(ctx context.Context, g *Graph, state *State, filter NodeFilter) (*Result, error) {
	start := time.Now()

	levels, err := BuildLevels(g)
	if err != nil {
		return nil, err
	}

	result := &Result{
		NodeResults: make(map[string]NodeResult),
	}

	for _, level := range levels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Determine which nodes to run in this level
		var toRun []string
		for _, name := range level {
			if filter != nil && !filter(name, state) {
				result.NodeResults[name] = NodeResult{
					Name:   name,
					Status: "skipped",
				}
				continue
			}
			toRun = append(toRun, name)
		}

		if len(toRun) == 0 {
			continue
		}

		// Execute nodes in this level concurrently
		e.executeLevel(ctx, g, state, toRun, result)
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (e *Engine) executeLevel(ctx context.Context, g *Graph, state *State, names []string, result *Result) {
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, e.concurrency(len(names)))

	for _, name := range names {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			nr := e.executeNode(ctx, g.Nodes[nodeName], state)
			mu.Lock()
			result.NodeResults[nodeName] = nr
			mu.Unlock()
		}(name)
	}

	wg.Wait()
}

func (e *Engine) executeNode(ctx context.Context, node Node, state *State) NodeResult {
	start := time.Now()
	output, err := node.Run(ctx, state)
	duration := time.Since(start)

	if err != nil {
		return NodeResult{
			Name:     node.Name(),
			Status:   "failed",
			Duration: duration,
			Error:    err,
		}
	}

	return NodeResult{
		Name:     node.Name(),
		Status:   "completed",
		Duration: duration,
		Output:   output,
	}
}

func (e *Engine) concurrency(levelSize int) int {
	if e.MaxParallel <= 0 || e.MaxParallel > levelSize {
		return levelSize
	}
	return e.MaxParallel
}

// AsTool wraps a DAG pipeline execution as a provider.RequestResponse.
// Input is written to state via InputFn, output is read via OutputFn.
func AsTool[I, O any](engine *Engine, graph *Graph, cfg ToolConfig[I, O]) *Tool[I, O] {
	return &Tool[I, O]{
		engine: engine,
		graph:  graph,
		cfg:    cfg,
	}
}

// ToolConfig configures how a DAG pipeline maps to a provider interface.
type ToolConfig[I, O any] struct {
	// Name is the provider name.
	Name string
	// InputFn writes input into state before execution.
	InputFn func(input I, state *State)
	// OutputFn reads output from state after execution.
	OutputFn func(state *State) (O, error)
}

// Tool wraps a DAG pipeline as a provider.RequestResponse.
type Tool[I, O any] struct {
	engine *Engine
	graph  *Graph
	cfg    ToolConfig[I, O]
}

func (t *Tool[I, O]) Name() string                       { return t.cfg.Name }
func (t *Tool[I, O]) IsAvailable(_ context.Context) bool { return true }

func (t *Tool[I, O]) Execute(ctx context.Context, input I) (O, error) {
	var zero O
	state := NewState()
	t.cfg.InputFn(input, state)

	_, err := t.engine.ExecuteBatch(ctx, t.graph, state)
	if err != nil {
		return zero, err
	}

	return t.cfg.OutputFn(state)
}
