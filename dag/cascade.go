package dag

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kbukum/gokit/provider"
)

// CascadeBuilder constructs a staged execution pipeline where each stage
// is a sub-DAG with its own configuration. Stages execute sequentially,
// with an advance condition between each pair of stages.
type CascadeBuilder[I, O any] struct {
	stages      []*stageSpec[I, O]
	finalStage  *stageSpec[I, O]
	onFailure   StageFailurePolicy
	mergeFunc   func(accumulated O, stageResult O) O
	orderFunc   OrderStrategy
	maxParallel int
}

// StageFunc is the function signature for stage builder callbacks.
type StageFunc[I, O any] func(b *StageBuilder[I, O], input I)

// StageFailurePolicy defines behavior when an entire stage fails.
type StageFailurePolicy int

const (
	// StageFailureAbort stops the cascade on stage failure.
	StageFailureAbort StageFailurePolicy = iota
	// StageFailureSkipToFinal skips to the final stage with accumulated results.
	StageFailureSkipToFinal
	// StageFailureContinue proceeds to the next stage regardless.
	StageFailureContinue
)

// SkipToFinal returns a policy that skips to the final stage on failure.
func SkipToFinal() StageFailurePolicy { return StageFailureSkipToFinal }

// Abort returns a policy that aborts the cascade on stage failure.
func Abort() StageFailurePolicy { return StageFailureAbort }

// ContinueOnFailure returns a policy that continues to the next stage on failure.
func ContinueOnFailure() StageFailurePolicy { return StageFailureContinue }

// OrderStrategy determines execution priority when multiple nodes
// within a stage are ready simultaneously and resources are constrained.
type OrderStrategy func(nodes []orderableNode) []orderableNode

type orderableNode struct {
	Name string
	Meta provider.Meta
}

// OrderByCost sorts nodes by the "cost" metadata key, cheapest first.
func OrderByCost() OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		sort.SliceStable(nodes, func(i, j int) bool {
			ci, _ := nodes[i].Meta.Float("cost")
			cj, _ := nodes[j].Meta.Float("cost")
			return ci < cj
		})
		return nodes
	}
}

// OrderByLatency sorts nodes by the "latency_ms" metadata key, fastest first.
func OrderByLatency() OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		sort.SliceStable(nodes, func(i, j int) bool {
			li, _ := nodes[i].Meta.Float("latency_ms")
			lj, _ := nodes[j].Meta.Float("latency_ms")
			return li < lj
		})
		return nodes
	}
}

// WeightedScore sorts nodes by a weighted combination of metadata dimensions.
// Higher weights mean that dimension matters more. Nodes with lower weighted
// scores execute first.
func WeightedScore(weights map[string]float64) OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		type scored struct {
			node  orderableNode
			score float64
		}
		items := make([]scored, len(nodes))
		for i, n := range nodes {
			var s float64
			for key, w := range weights {
				v, ok := n.Meta.Float(key)
				if ok {
					s += v * w
				}
			}
			items[i] = scored{node: n, score: s}
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].score < items[j].score
		})
		for i, item := range items {
			nodes[i] = item.node
		}
		return nodes
	}
}

// NewCascade creates a new cascade builder.
func NewCascade[I, O any]() *CascadeBuilder[I, O] {
	return &CascadeBuilder[I, O]{
		onFailure: StageFailureAbort,
	}
}

// Stage adds a named stage to the cascade. The builder function receives the
// input and can conditionally add nodes based on it.
func (c *CascadeBuilder[I, O]) Stage(name string, fn StageFunc[I, O]) *CascadeBuilder[I, O] {
	c.stages = append(c.stages, &stageSpec[I, O]{
		name:    name,
		buildFn: fn,
	})
	return c
}

// FinalStage sets the final stage that always executes with all accumulated
// results, regardless of early exit.
func (c *CascadeBuilder[I, O]) FinalStage(name string, fn StageFunc[I, O]) *CascadeBuilder[I, O] {
	c.finalStage = &stageSpec[I, O]{
		name:    name,
		buildFn: fn,
	}
	return c
}

// OnStageFailure sets the policy for when an entire stage fails.
func (c *CascadeBuilder[I, O]) OnStageFailure(policy StageFailurePolicy) *CascadeBuilder[I, O] {
	c.onFailure = policy
	return c
}

// MergeStrategy sets how results from each stage are accumulated.
func (c *CascadeBuilder[I, O]) MergeStrategy(fn func(accumulated O, stageResult O) O) *CascadeBuilder[I, O] {
	c.mergeFunc = fn
	return c
}

// OrderNodesBy sets the ordering strategy for parallel nodes within a stage.
func (c *CascadeBuilder[I, O]) OrderNodesBy(strategy OrderStrategy) *CascadeBuilder[I, O] {
	c.orderFunc = strategy
	return c
}

// MaxConcurrency limits parallel node execution within each stage.
func (c *CascadeBuilder[I, O]) MaxConcurrency(n int) *CascadeBuilder[I, O] {
	c.maxParallel = n
	return c
}

// Build creates an executable Cascade.
func (c *CascadeBuilder[I, O]) Build() *Cascade[I, O] {
	return &Cascade[I, O]{
		stages:      c.stages,
		finalStage:  c.finalStage,
		onFailure:   c.onFailure,
		mergeFunc:   c.mergeFunc,
		orderFunc:   c.orderFunc,
		maxParallel: c.maxParallel,
	}
}

// stageSpec holds the definition for a single stage.
type stageSpec[I, O any] struct {
	name    string
	buildFn StageFunc[I, O]
}

// Cascade is an executable staged pipeline.
type Cascade[I, O any] struct {
	stages      []*stageSpec[I, O]
	finalStage  *stageSpec[I, O]
	onFailure   StageFailurePolicy
	mergeFunc   func(accumulated O, stageResult O) O
	orderFunc   OrderStrategy
	maxParallel int
}

// CascadeTrace holds execution details for observability.
type CascadeTrace struct {
	StagesExecuted []string                    `json:"stages_executed"`
	StagesSkipped  []string                    `json:"stages_skipped"`
	NodeResults    map[string]CascadeNodeTrace `json:"node_results"`
	TotalDuration  time.Duration               `json:"total_duration"`
	TotalCost      float64                     `json:"total_cost"`
	EarlyExit      bool                        `json:"early_exit"`
	ExitedAtStage  string                      `json:"exited_at_stage,omitempty"`
	Error          error                       `json:"-"`
}

// CascadeNodeTrace holds per-node execution details.
type CascadeNodeTrace struct {
	Name     string        `json:"name"`
	Stage    string        `json:"stage"`
	Duration time.Duration `json:"duration"`
	Status   string        `json:"status"`
	Meta     provider.Meta `json:"meta,omitempty"`
	Error    error         `json:"-"`
}

// Execute runs the cascade: stages execute sequentially, nodes within each
// stage execute concurrently, and advance conditions control stage progression.
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

		// Check advance condition — if AdvanceWhen was set and returns false, stop.
		sb := c.buildStage(spec, input)
		if sb.advanceFn != nil && !sb.advanceFn(accumulated) {
			exitedEarly = true
			trace.EarlyExit = true
			trace.ExitedAtStage = spec.name
		}
	}

	// Execute final stage if defined.
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

func (c *Cascade[I, O]) buildStage(spec *stageSpec[I, O], input I) *StageBuilder[I, O] {
	sb := &StageBuilder[I, O]{name: spec.name}
	spec.buildFn(sb, input)
	return sb
}

func (c *Cascade[I, O]) executeStage(
	ctx context.Context,
	spec *stageSpec[I, O],
	input I,
) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	sb := c.buildStage(spec, input)
	traces := make(map[string]CascadeNodeTrace)

	if len(sb.nodes) == 0 {
		var zero O
		return zero, traces, nil
	}

	// Apply stage timeout.
	stageCtx := ctx
	if sb.timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, sb.timeout)
		defer cancel()
	}

	// Order nodes if strategy is set.
	nodes := sb.nodes
	if c.orderFunc != nil {
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
		nodes = ordered
	}

	// Separate nodes with dependencies from independent ones.
	hasEdges := len(sb.edges) > 0
	if hasEdges {
		return c.executeStageWithEdges(stageCtx, spec.name, sb, input, traces)
	}

	// Simple parallel execution (no internal edges).
	return c.executeStageParallel(stageCtx, spec.name, nodes, sb, input, traces)
}

func (c *Cascade[I, O]) executeStageParallel(
	ctx context.Context,
	stageName string,
	nodes []*cascadeNode[I, O],
	sb *StageBuilder[I, O],
	input I,
	traces map[string]CascadeNodeTrace,
) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	type nodeResult struct {
		name   string
		result O
		err    error
		dur    time.Duration
		meta   provider.Meta
	}

	maxParallel := c.maxParallel
	if maxParallel <= 0 {
		maxParallel = len(nodes)
	}

	// If sequential (MaxConcurrency=1), execute in order without goroutines.
	if maxParallel == 1 {
		return c.executeStageSequential(ctx, stageName, nodes, sb, input, traces)
	}

	sem := make(chan struct{}, maxParallel)

	results := make([]nodeResult, len(nodes))
	var wg sync.WaitGroup

	for i, n := range nodes {
		wg.Add(1)
		go func(idx int, node *cascadeNode[I, O]) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			result, err := node.execute(ctx, input)
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
		status := StatusCompleted
		if r.err != nil {
			status = StatusFailed
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

		traces[r.name] = CascadeNodeTrace{
			Name:     r.name,
			Stage:    stageName,
			Duration: r.dur,
			Status:   status,
			Meta:     r.meta,
			Error:    r.err,
		}
	}

	if allFailed && len(nodes) > 0 {
		if sb.failurePolicy == StageContinueWithPartial {
			return accumulated, traces, nil
		}
		return accumulated, traces, firstErr
	}

	return accumulated, traces, nil
}

func (c *Cascade[I, O]) executeStageSequential(
	ctx context.Context,
	stageName string,
	nodes []*cascadeNode[I, O],
	sb *StageBuilder[I, O],
	input I,
	traces map[string]CascadeNodeTrace,
) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	var accumulated O
	var firstErr error
	hasResult := false
	allFailed := true

	for _, node := range nodes {
		if ctx.Err() != nil {
			traces[node.name] = CascadeNodeTrace{
				Name:   node.name,
				Stage:  stageName,
				Status: StatusSkipped,
				Meta:   node.meta,
			}
			continue
		}

		start := time.Now()
		result, err := node.execute(ctx, input)
		dur := time.Since(start)

		status := StatusCompleted
		if err != nil {
			status = StatusFailed
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

		traces[node.name] = CascadeNodeTrace{
			Name:     node.name,
			Stage:    stageName,
			Duration: dur,
			Status:   status,
			Meta:     node.meta,
			Error:    err,
		}
	}

	if allFailed && len(nodes) > 0 {
		if sb.failurePolicy == StageContinueWithPartial {
			return accumulated, traces, nil
		}
		return accumulated, traces, firstErr
	}

	return accumulated, traces, nil
}

func (c *Cascade[I, O]) executeStageWithEdges(
	ctx context.Context,
	stageName string,
	sb *StageBuilder[I, O],
	input I,
	traces map[string]CascadeNodeTrace,
) (result O, nodeTraces map[string]CascadeNodeTrace, err error) {
	// Build a mini-graph from the stage's nodes and edges.
	nodeMap := make(map[string]*cascadeNode[I, O])
	for _, n := range sb.nodes {
		nodeMap[n.name] = n
	}

	// Build dependency levels using topological sort.
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	for _, n := range sb.nodes {
		if _, exists := inDegree[n.name]; !exists {
			inDegree[n.name] = 0
		}
	}
	for _, e := range sb.edges {
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

		levelResult, levelTraces, err := c.executeStageParallel(ctx, stageName, levelNodes, sb, input, traces)
		for k, v := range levelTraces {
			traces[k] = v
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

	if firstErr != nil && sb.failurePolicy != StageContinueWithPartial {
		return accumulated, traces, firstErr
	}

	return accumulated, traces, nil
}

func (c *Cascade[I, O]) computeTotalCost(trace *CascadeTrace) {
	for _, nt := range trace.NodeResults {
		if cost, ok := nt.Meta.Float("cost"); ok {
			trace.TotalCost += cost
		}
	}
}

// StageBuilder lets you configure a single stage within a cascade.
type StageBuilder[I, O any] struct {
	name          string
	nodes         []*cascadeNode[I, O]
	edges         []cascadeEdge
	timeout       time.Duration
	advanceFn     func(O) bool
	failurePolicy StageNodeFailurePolicy
}

// StageNodeFailurePolicy defines behavior when nodes within a stage fail.
type StageNodeFailurePolicy int

const (
	// StageAbortOnFailure stops the stage if any node fails.
	StageAbortOnFailure StageNodeFailurePolicy = iota
	// StageContinueWithPartial continues with partial results on node failure.
	StageContinueWithPartial
)

// ContinueWithPartial returns a policy that continues with partial results.
func ContinueWithPartial() StageNodeFailurePolicy { return StageContinueWithPartial }

// AddNode adds a provider-backed node to this stage.
func (b *StageBuilder[I, O]) AddNode(name string, p provider.RequestResponse[I, O]) {
	meta := provider.GetMeta(p)
	b.nodes = append(b.nodes, &cascadeNode[I, O]{
		name:     name,
		provider: p,
		meta:     meta,
	})
}

// Edge adds a dependency edge within this stage.
// "from" must complete before "to" starts.
func (b *StageBuilder[I, O]) Edge(from, to string) {
	b.edges = append(b.edges, cascadeEdge{from: from, to: to})
}

// Timeout sets the maximum duration for this stage.
func (b *StageBuilder[I, O]) Timeout(d time.Duration) {
	b.timeout = d
}

// AdvanceWhen sets the condition for advancing to the next stage.
// If the function returns false, the cascade stops after this stage
// (early exit). The function receives the accumulated result so far.
func (b *StageBuilder[I, O]) AdvanceWhen(fn func(O) bool) {
	b.advanceFn = fn
}

// OnFailure sets the node failure policy for this stage.
func (b *StageBuilder[I, O]) OnFailure(policy StageNodeFailurePolicy) {
	b.failurePolicy = policy
}

type cascadeNode[I, O any] struct {
	name     string
	provider provider.RequestResponse[I, O]
	meta     provider.Meta
}

func (n *cascadeNode[I, O]) execute(ctx context.Context, input I) (O, error) {
	return n.provider.Execute(ctx, input)
}

type cascadeEdge struct {
	from string
	to   string
}
