package discovery

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Balancer selects one instance from a candidate set.
//
// Implementations must be safe for concurrent use. Pick returns the zero
// ServiceInstance and false when the candidate slice is empty.
type Balancer interface {
	Pick(instances []ServiceInstance) (ServiceInstance, bool)
}

// NewBalancer returns the Balancer for a load-balancing strategy. Unknown
// strategies fall back to random selection.
func NewBalancer(strategy LoadBalancingStrategy) Balancer {
	switch strategy {
	case StrategyRoundRobin:
		return NewRoundRobinBalancer()
	case StrategyWeighted:
		return NewWeightedBalancer()
	case StrategyLeastConn:
		return NewLeastConnectionsBalancer()
	default:
		return NewRandomBalancer()
	}
}

// RoundRobinBalancer cycles through instances in order.
type RoundRobinBalancer struct {
	counter atomic.Uint64
}

// NewRoundRobinBalancer creates a round-robin balancer.
func NewRoundRobinBalancer() *RoundRobinBalancer { return &RoundRobinBalancer{} }

// Pick returns the next instance in rotation.
func (b *RoundRobinBalancer) Pick(instances []ServiceInstance) (ServiceInstance, bool) {
	if len(instances) == 0 {
		return ServiceInstance{}, false
	}
	idx := b.counter.Add(1) - 1
	return instances[int(idx%uint64(len(instances)))], true
}

// RandomBalancer picks a uniformly random instance each call.
type RandomBalancer struct {
	mu sync.Mutex
	r  *rand.Rand
}

// NewRandomBalancer creates a random balancer seeded from the current time.
func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{r: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// Pick returns a random instance.
func (b *RandomBalancer) Pick(instances []ServiceInstance) (ServiceInstance, bool) {
	if len(instances) == 0 {
		return ServiceInstance{}, false
	}
	b.mu.Lock()
	idx := b.r.Intn(len(instances))
	b.mu.Unlock()
	return instances[idx], true
}

// WeightedBalancer picks an instance at random in proportion to its Weight.
// A non-positive weight is treated as 1.
type WeightedBalancer struct {
	mu sync.Mutex
	r  *rand.Rand
}

// NewWeightedBalancer creates a weighted balancer seeded from the current time.
func NewWeightedBalancer() *WeightedBalancer {
	return &WeightedBalancer{r: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// Pick returns a weight-proportional random instance.
func (b *WeightedBalancer) Pick(instances []ServiceInstance) (ServiceInstance, bool) {
	if len(instances) == 0 {
		return ServiceInstance{}, false
	}

	total := 0
	for i := range instances {
		total += effectiveWeight(instances[i].Weight)
	}

	b.mu.Lock()
	slot := b.r.Intn(total)
	b.mu.Unlock()

	for i := range instances {
		slot -= effectiveWeight(instances[i].Weight)
		if slot < 0 {
			return instances[i], true
		}
	}
	return instances[len(instances)-1], true
}

// LeastConnectionsBalancer tracks in-flight requests per instance and picks the
// one with the fewest. Callers bracket a selected instance with Acquire before
// use and Release when done so the counts reflect live load.
type LeastConnectionsBalancer struct {
	mu       sync.Mutex
	inFlight map[string]int
}

// NewLeastConnectionsBalancer creates a least-connections balancer.
func NewLeastConnectionsBalancer() *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{inFlight: make(map[string]int)}
}

// Acquire increments the in-flight count for an instance ID.
func (b *LeastConnectionsBalancer) Acquire(id string) {
	b.mu.Lock()
	b.inFlight[id]++
	b.mu.Unlock()
}

// Release decrements the in-flight count for an instance ID, never below zero.
func (b *LeastConnectionsBalancer) Release(id string) {
	b.mu.Lock()
	if b.inFlight[id] > 0 {
		b.inFlight[id]--
	}
	if b.inFlight[id] == 0 {
		delete(b.inFlight, id)
	}
	b.mu.Unlock()
}

// Pick returns the instance with the fewest in-flight requests. Ties resolve to
// the earliest instance in the slice for stable selection.
func (b *LeastConnectionsBalancer) Pick(instances []ServiceInstance) (ServiceInstance, bool) {
	if len(instances) == 0 {
		return ServiceInstance{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	best := 0
	bestCount := b.inFlight[instances[0].ID]
	for i := 1; i < len(instances); i++ {
		if c := b.inFlight[instances[i].ID]; c < bestCount {
			best = i
			bestCount = c
		}
	}
	return instances[best], true
}

func effectiveWeight(w int) int {
	if w <= 0 {
		return 1
	}
	return w
}
