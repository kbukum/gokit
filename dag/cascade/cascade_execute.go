package cascade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/dag/status"

	"github.com/kbukum/gokit/provider"
)

// Execute runs the cascade: stages execute sequentially, nodes within each stage execute concurrently, and advance conditions control stage progression.
func (c *Cascade[I, O]) Execute(ctx context.Context, input I) (O, *CascadeTrace) {
	start := time.Now()
	trace := &CascadeTrace{
		NodeResults: make(map[string]CascadeNodeTrace),
	}

	var accumulated O
	exitedEarly := false

	for _, spec := range c.stages {
		if ctx.Err() != nil {
			trace.StagesSkipped = append(trace.StagesSkipped, spec.name)
			continue
		}

		if exitedEarly {
			trace.StagesSkipped = append(trace.StagesSkipped, spec.name)
			continue
		}

		stageResult, stageNodeTraces, err := c.executeStage(ctx, spec, input)
		for k, v := range stageNodeTraces {
			trace.NodeResults[k] = v
		}

		if err != nil {
			trace.StagesExecuted = append(trace.StagesExecuted, spec.name)
			switch c.onFailure {
			case StageFailureAbort:
				trace.Error = fmt.Errorf("stage %q failed: %w", spec.name, err)
				trace.TotalDuration = time.Since(start)
				c.computeTotalCost(trace)
				return accumulated, trace
			case StageFailureSkipToFinal:
				exitedEarly = true
				continue
			case StageFailureContinue:
				continue
			}
		}

		trace.StagesExecuted = append(trace.StagesExecuted, spec.name)

		if c.mergeFunc != nil {
			accumulated = c.mergeFunc(accumulated, stageResult)
		} else {
			accumulated = stageResult
		}

		sb := c.buildStage(spec, input)
		if sb.advanceFn != nil && !sb.advanceFn(accumulated) {
			exitedEarly = true
			trace.EarlyExit = true
			trace.ExitedAtStage = spec.name
		}
	}

	if c.finalStage != nil && ctx.Err() == nil {
		finalResult, finalTraces, err := c.executeStage(ctx, c.finalStage, input)
		for k, v := range finalTraces {
			trace.NodeResults[k] = v
		}
		trace.StagesExecuted = append(trace.StagesExecuted, c.finalStage.name)

		if err == nil {
			if c.mergeFunc != nil {
				accumulated = c.mergeFunc(accumulated, finalResult)
			} else {
				accumulated = finalResult
			}
		} else {
			trace.Error = fmt.Errorf("final stage %q failed: %w", c.finalStage.name, err)
		}
	}

	trace.TotalDuration = time.Since(start)
	c.computeTotalCost(trace)
	return accumulated, trace
}

type stageExecution[I, O any] struct {
	name   string
	nodes  []*cascadeNode[I, O]
	sb     *StageBuilder[I, O]
	input  I
	traces map[string]CascadeNodeTrace
}

func (c *Cascade[I, O]) buildStage(spec *stageSpec[I, O], input I) *StageBuilder[I, O] {
	sb := &StageBuilder[I, O]{name: spec.name}
	spec.buildFn(sb, input)
	return sb
}

func (c *Cascade[I, O]) executeStage(ctx context.Context, spec *stageSpec[I, O], input I) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	sb := c.buildStage(spec, input)
	traces := make(map[string]CascadeNodeTrace)

	if len(sb.nodes) == 0 {
		var zero O
		return zero, traces, nil
	}

	stageCtx := ctx
	if sb.timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, sb.timeout)
		defer cancel()
	}

	exec := stageExecution[I, O]{
		name:   spec.name,
		nodes:  c.orderStageNodes(sb.nodes),
		sb:     sb,
		input:  input,
		traces: traces,
	}
	if len(sb.edges) > 0 {
		return c.executeStageWithEdges(stageCtx, exec)
	}
	return c.executeStageParallel(stageCtx, exec)
}

func (c *Cascade[I, O]) orderStageNodes(nodes []*cascadeNode[I, O]) []*cascadeNode[I, O] {
	if c.orderFunc == nil {
		return nodes
	}

	orderables := make([]orderableNode, len(nodes))
	for i, n := range nodes {
		orderables[i] = orderableNode{Name: n.name, Meta: n.meta}
	}
	orderables = c.orderFunc(orderables)
	ordered := make([]*cascadeNode[I, O], len(nodes))
	nodeMap := make(map[string]*cascadeNode[I, O])
	for _, n := range nodes {
		nodeMap[n.name] = n
	}
	for i, o := range orderables {
		ordered[i] = nodeMap[o.Name]
	}
	return ordered
}

func (c *Cascade[I, O]) executeStageParallel(ctx context.Context, exec stageExecution[I, O]) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	type nodeResult struct {
		name   string
		result O
		err    error
		dur    time.Duration
		meta   provider.Meta
	}

	maxParallel := c.maxParallel
	if maxParallel <= 0 {
		maxParallel = len(exec.nodes)
	}

	if maxParallel == 1 {
		return c.executeStageSequential(ctx, exec)
	}

	sem := make(chan struct{}, maxParallel)
	results := make([]nodeResult, len(exec.nodes))
	var wg sync.WaitGroup

	for i, n := range exec.nodes {
		wg.Add(1)
		go func(idx int, node *cascadeNode[I, O]) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			result, err := node.execute(ctx, exec.input)
			dur := time.Since(start)

			results[idx] = nodeResult{
				name:   node.name,
				result: result,
				err:    err,
				dur:    dur,
				meta:   node.meta,
			}
		}(i, n)
	}
	wg.Wait()

	var accumulated O
	var firstErr error
	hasResult := false
	allFailed := true

	for _, r := range results {
		nodeStatus := status.Completed
		if r.err != nil {
			nodeStatus = status.Failed
			if firstErr == nil {
				firstErr = r.err
			}
		} else {
			allFailed = false
			if c.mergeFunc != nil && hasResult {
				accumulated = c.mergeFunc(accumulated, r.result)
			} else {
				accumulated = r.result
				hasResult = true
			}
		}

		exec.traces[r.name] = CascadeNodeTrace{
			Name:     r.name,
			Stage:    exec.name,
			Duration: r.dur,
			Status:   nodeStatus,
			Meta:     r.meta,
			Error:    r.err,
		}
	}

	if allFailed && len(exec.nodes) > 0 {
		if exec.sb.failurePolicy == StageContinueWithPartial {
			return accumulated, exec.traces, nil
		}
		return accumulated, exec.traces, firstErr
	}

	return accumulated, exec.traces, nil
}

func (c *Cascade[I, O]) executeStageSequential(ctx context.Context, exec stageExecution[I, O]) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	var accumulated O
	var firstErr error
	hasResult := false
	allFailed := true

	for _, node := range exec.nodes {
		if ctx.Err() != nil {
			exec.traces[node.name] = CascadeNodeTrace{
				Name:   node.name,
				Stage:  exec.name,
				Status: status.Skipped,
				Meta:   node.meta,
			}
			continue
		}

		start := time.Now()
		result, err := node.execute(ctx, exec.input)
		dur := time.Since(start)

		nodeStatus := status.Completed
		if err != nil {
			nodeStatus = status.Failed
			if firstErr == nil {
				firstErr = err
			}
		} else {
			allFailed = false
			if c.mergeFunc != nil && hasResult {
				accumulated = c.mergeFunc(accumulated, result)
			} else {
				accumulated = result
				hasResult = true
			}
		}

		exec.traces[node.name] = CascadeNodeTrace{
			Name:     node.name,
			Stage:    exec.name,
			Duration: dur,
			Status:   nodeStatus,
			Meta:     node.meta,
			Error:    err,
		}
	}

	if allFailed && len(exec.nodes) > 0 {
		if exec.sb.failurePolicy == StageContinueWithPartial {
			return accumulated, exec.traces, nil
		}
		return accumulated, exec.traces, firstErr
	}

	return accumulated, exec.traces, nil
}

func (c *Cascade[I, O]) executeStageWithEdges(ctx context.Context, exec stageExecution[I, O]) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	nodeMap := make(map[string]*cascadeNode[I, O])
	for _, n := range exec.sb.nodes {
		nodeMap[n.name] = n
	}

	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	for _, n := range exec.sb.nodes {
		if _, exists := inDegree[n.name]; !exists {
			inDegree[n.name] = 0
		}
	}
	for _, e := range exec.sb.edges {
		inDegree[e.to]++
		dependents[e.from] = append(dependents[e.from], e.to)
	}

	var levels [][]string
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	for len(queue) > 0 {
		levels = append(levels, queue)
		var next []string
		for _, n := range queue {
			for _, dep := range dependents[n] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		queue = next
	}

	var accumulated O
	hasResult := false
	var firstErr error

	for _, level := range levels {
		levelNodes := make([]*cascadeNode[I, O], 0, len(level))
		for _, name := range level {
			if n, ok := nodeMap[name]; ok {
				levelNodes = append(levelNodes, n)
			}
		}

		levelExec := exec
		levelExec.nodes = levelNodes
		levelResult, levelTraces, err := c.executeStageParallel(ctx, levelExec)
		for k, v := range levelTraces {
			exec.traces[k] = v
		}

		if err != nil && firstErr == nil {
			firstErr = err
		}
		if err == nil {
			if c.mergeFunc != nil && hasResult {
				accumulated = c.mergeFunc(accumulated, levelResult)
			} else {
				accumulated = levelResult
				hasResult = true
			}
		}
	}

	if firstErr != nil && exec.sb.failurePolicy != StageContinueWithPartial {
		return accumulated, exec.traces, firstErr
	}

	return accumulated, exec.traces, nil
}
