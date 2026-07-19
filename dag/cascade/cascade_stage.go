package cascade

import (
	"context"
	"time"

	"github.com/kbukum/gokit/provider"
)

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

// Edge adds a dependency edge within this stage. "from" must complete before "to" starts.
func (b *StageBuilder[I, O]) Edge(from, to string) {
	b.edges = append(b.edges, cascadeEdge{from: from, to: to})
}

// Timeout sets the maximum duration for this stage.
func (b *StageBuilder[I, O]) Timeout(d time.Duration) {
	b.timeout = d
}

// AdvanceWhen sets the condition for advancing to the next stage. If the function returns false, the cascade stops after this stage (early exit). The function receives the accumulated result so far.
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
