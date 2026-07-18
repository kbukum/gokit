package worker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RestartPolicy controls when a crashed worker should be restarted.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"
	RestartOnFailure RestartPolicy = "on_failure"
	RestartAlways    RestartPolicy = "always"
)

// SupervisorConfig configures worker supervision.
type SupervisorConfig struct {
	RestartPolicy  RestartPolicy `yaml:"restart_policy"  mapstructure:"restart_policy"`  // never | on_failure | always
	MaxRestarts    int           `yaml:"max_restarts"    mapstructure:"max_restarts"`    // 0 = unlimited
	BackoffBase    time.Duration `yaml:"backoff_base"    mapstructure:"backoff_base"`    // exponential backoff base (default: 1s)
	HealthInterval time.Duration `yaml:"health_interval" mapstructure:"health_interval"` // health check frequency (default: 30s)
}

func (c SupervisorConfig) withDefaults() SupervisorConfig {
	if c.BackoffBase <= 0 {
		c.BackoffBase = time.Second
	}
	if c.HealthInterval <= 0 {
		c.HealthInterval = 30 * time.Second
	}
	return c
}

// supervisor monitors worker health, tracks per-worker panic counts, and enforces failure policies.
//
// Design: worker goroutines catch panics per-task and survive —
// the goroutine keeps processing the next task from its channel.
// The supervisor tracks failure counts per worker
// and can mark a worker as unhealthy when it exceeds MaxRestarts.
// This avoids the complexity of goroutine replacement while still providing supervision visibility
// and policy enforcement.
type supervisor[I, O any] struct {
	pool *Pool[I, O]
	cfg  SupervisorConfig

	mu     sync.Mutex
	panics []int  // per-worker cumulative panic count
	alive  []bool // per-worker health status
}

func newSupervisor[I, O any](pool *Pool[I, O], cfg SupervisorConfig) *supervisor[I, O] {
	alive := make([]bool, pool.cfg.Size)
	for i := range alive {
		alive[i] = true
	}
	return &supervisor[I, O]{
		pool:   pool,
		cfg:    cfg.withDefaults(),
		panics: make([]int, pool.cfg.Size),
		alive:  alive,
	}
}

// run is the supervisor loop that periodically checks worker health.
func (s *supervisor[I, O]) run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.healthCheck()
		}
	}
}

// reportCrash is called by a worker goroutine when a task panics.
// The worker goroutine itself survives (panic is caught per-task in executeTask).
// The supervisor tracks the crash count and marks the worker unhealthy when it exceeds MaxRestarts.
func (s *supervisor[I, O]) reportCrash(workerIdx int, reason any) {
	s.mu.Lock()
	s.panics[workerIdx]++
	count := s.panics[workerIdx]
	exceeded := s.cfg.MaxRestarts > 0 && count >= s.cfg.MaxRestarts
	if exceeded {
		s.alive[workerIdx] = false
	}
	s.mu.Unlock()

	s.emitLog(fmt.Sprintf("worker %s-w%d task panic (%d total): %v",
		s.pool.cfg.Name, workerIdx, count, reason))

	if exceeded {
		s.emitLog(fmt.Sprintf("worker %s-w%d exceeded max panics (%d), marked unhealthy",
			s.pool.cfg.Name, workerIdx, s.cfg.MaxRestarts))
	}
}

// shouldAcceptTask returns whether a supervised worker should process tasks.
// Workers marked unhealthy by the supervisor are skipped by dispatch.
func (s *supervisor[I, O]) shouldAcceptTask(workerIdx int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.RestartPolicy == RestartNever && s.panics[workerIdx] > 0 {
		return false
	}
	return s.alive[workerIdx]
}

// healthCheck logs the current health status of all workers.
func (s *supervisor[I, O]) healthCheck() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var dead int
	for _, a := range s.alive {
		if !a {
			dead++
		}
	}
	if dead > 0 {
		s.emitLog(fmt.Sprintf("health: %d/%d workers unhealthy in pool %s",
			dead, len(s.alive), s.pool.cfg.Name))
	}
}

// backoff returns the exponential backoff duration for a worker's Nth panic.
// Returns 0 if no panics have occurred. Caps at 30 seconds.
func (s *supervisor[I, O]) backoff(workerIdx int) time.Duration {
	s.mu.Lock()
	n := s.panics[workerIdx]
	s.mu.Unlock()

	if n == 0 {
		return 0
	}

	d := s.cfg.BackoffBase
	for range n - 1 {
		d *= 2
		if d > 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}

// emitLog sends a log event through the pool's aggregated event channel.
func (s *supervisor[I, O]) emitLog(msg string) {
	e := LogEvent[O](msg, map[string]any{"source": "supervisor"})
	e.WorkerID = "supervisor"
	select {
	case s.pool.events <- e:
	default:
	}
}
