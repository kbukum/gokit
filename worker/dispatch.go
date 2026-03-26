package worker

import "sync/atomic"

// DispatchStrategy controls how tasks are assigned to workers.
type DispatchStrategy string

const (
	RoundRobin  DispatchStrategy = "round_robin"  // rotate through workers sequentially
	LeastLoaded DispatchStrategy = "least_loaded" // pick the worker with fewest active tasks
)

// dispatcher selects a worker index for the next task.
// Implementations must be safe for concurrent use.
type dispatcher interface {
	next(stats []workerStats) int
}

// workerStats tracks per-worker load for dispatch decisions.
type workerStats struct {
	active atomic.Int32 // number of tasks currently executing
}

func newDispatcher(strategy DispatchStrategy) dispatcher {
	switch strategy {
	case LeastLoaded:
		return &leastLoadedDispatcher{}
	default:
		return &roundRobinDispatcher{}
	}
}

// roundRobinDispatcher cycles through workers in order.
// Uses atomic counter for lock-free concurrent access.
type roundRobinDispatcher struct {
	counter atomic.Uint64
}

func (d *roundRobinDispatcher) next(stats []workerStats) int {
	n := d.counter.Add(1) - 1
	return int(n % uint64(len(stats)))
}

// leastLoadedDispatcher picks the worker with the fewest active tasks.
// Caller must ensure stats is a consistent snapshot (e.g., copied under lock).
type leastLoadedDispatcher struct{}

func (d *leastLoadedDispatcher) next(stats []workerStats) int {
	minIdx := 0
	minLoad := stats[0].active.Load()
	for i := 1; i < len(stats); i++ {
		if load := stats[i].active.Load(); load < minLoad {
			minLoad = load
			minIdx = i
		}
	}
	return minIdx
}
