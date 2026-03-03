package dag

import (
	"context"
	"errors"
	"fmt"
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

	// Build upstream dependency map: node → list of nodes it depends on
	upstreams := make(map[string][]string)
	for _, edge := range g.Edges {
		upstreams[edge.To] = append(upstreams[edge.To], edge.From)
	}

	result := &Result{
		NodeResults: make(map[string]NodeResult),
	}

	for _, level := range levels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var toRun []string
		for _, name := range level {
			// Check upstream dependency results from this cycle
			if skipStatus, shouldSkip := e.checkUpstreams(name, upstreams, result, g); shouldSkip {
				result.NodeResults[name] = NodeResult{
					Name:   name,
					Status: skipStatus,
				}
				continue
			}

			// Apply schedule/condition filter
			if filter != nil && !filter(name, state) {
				result.NodeResults[name] = NodeResult{
					Name:   name,
					Status: StatusSkipped,
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

		// Check for on_error=fail after each level
		for _, name := range toRun {
			nr, ok := result.NodeResults[name]
			if !ok {
				continue
			}
			if (nr.Status == StatusFailed || nr.Status == StatusUnavailable) && g.GetNodeDef(name).EffectiveOnError() == OnErrorFail {
				result.Duration = time.Since(start)
				return nil, fmt.Errorf("dag: node %q failed with on_error=fail: %w", name, nr.Error)
			}
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// checkUpstreams examines this cycle's results for all upstream dependencies.
// Returns (skipStatus, true) if the node should be skipped, or ("", false) to proceed.
func (e *Engine) checkUpstreams(name string, upstreams map[string][]string, result *Result, g *Graph) (string, bool) {
	for _, upstream := range upstreams[name] {
		ur, exists := result.NodeResults[upstream]
		if !exists {
			continue // upstream not yet processed or not in this cycle
		}

		policy := g.GetNodeDef(upstream).EffectiveOnError()

		switch {
		case ur.Status == StatusUnavailable || ur.Status == StatusDepUnavailable:
			if policy != OnErrorContinue {
				return StatusDepUnavailable, true
			}
		case ur.Status == StatusFailed || ur.Status == StatusDepFailed:
			if policy != OnErrorContinue {
				return StatusDepFailed, true
			}
		}
	}
	return "", false
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
		status := StatusFailed
		if errors.Is(err, ErrUnavailable) {
			status = StatusUnavailable
		}
		return NodeResult{
			Name:     node.Name(),
			Status:   status,
			Duration: duration,
			Error:    err,
		}
	}

	return NodeResult{
		Name:     node.Name(),
		Status:   StatusCompleted,
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
