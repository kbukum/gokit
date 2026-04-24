package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kbukum/gokit/component"
)

// Job defines a single periodic task managed by a Scheduler.
type Job struct {
	// Name identifies the job in health reports and logs.
	Name string
	// Interval between consecutive runs.
	Interval time.Duration
	// RunOnStart causes the job to execute once immediately when the
	// scheduler starts, before entering its periodic loop.
	RunOnStart bool
	// Fn is the work to perform on each tick.
	Fn TickerFunc
}

// Scheduler is a Component that manages multiple periodic jobs. Each
// job runs in its own goroutine via an internal TickerWorker, giving
// independent intervals, health, and non-overlap guarantees.
//
// Scheduler implements component.Component and component.Describable.
//
// Example:
//
//	s := worker.NewScheduler("background-jobs",
//	    worker.Job{Name: "catalog-refresh", Interval: 6 * time.Hour, RunOnStart: true, Fn: refreshFn},
//	    worker.Job{Name: "cleanup", Interval: 24 * time.Hour, Fn: cleanupFn},
//	)
//	registry.Register(s)
type Scheduler struct {
	name    string
	workers []*TickerWorker
}

// NewScheduler creates a Scheduler with the given name and jobs.
func NewScheduler(name string, jobs ...Job) *Scheduler {
	workers := make([]*TickerWorker, 0, len(jobs))
	for _, j := range jobs {
		var opts []TickerOption
		if j.RunOnStart {
			opts = append(opts, WithRunOnStart())
		}
		workers = append(workers, NewTickerWorker(j.Name, j.Interval, j.Fn, opts...))
	}
	return &Scheduler{name: name, workers: workers}
}

// Name returns the scheduler's component name.
func (s *Scheduler) Name() string { return s.name }

// Start launches all jobs. If any job fails to start, previously
// started jobs are stopped and the first error is returned.
func (s *Scheduler) Start(ctx context.Context) error {
	for i, w := range s.workers {
		if err := w.Start(ctx); err != nil {
			// Roll back already-started workers.
			for j := i - 1; j >= 0; j-- {
				_ = s.workers[j].Stop(ctx)
			}
			return fmt.Errorf("scheduler %s: failed to start job %s: %w", s.name, w.Name(), err)
		}
	}
	return nil
}

// Stop signals all jobs to exit and waits for each to finish.
func (s *Scheduler) Stop(ctx context.Context) error {
	var firstErr error
	for i := len(s.workers) - 1; i >= 0; i-- {
		if err := s.workers[i].Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Health aggregates health from all jobs. The scheduler is healthy only
// if every job is healthy. If any job is degraded or unhealthy, the
// scheduler reports the worst status.
func (s *Scheduler) Health(ctx context.Context) component.Health {
	if len(s.workers) == 0 {
		return component.Health{Name: s.name, Status: component.StatusHealthy, Message: "no jobs"}
	}
	worst := component.StatusHealthy
	var msgs []string
	for _, w := range s.workers {
		h := w.Health(ctx)
		if statusSeverity(h.Status) > statusSeverity(worst) {
			worst = h.Status
		}
		if h.Status != component.StatusHealthy {
			msgs = append(msgs, fmt.Sprintf("%s: %s", w.Name(), h.Message))
		}
	}
	msg := fmt.Sprintf("%d jobs running", len(s.workers))
	if len(msgs) > 0 {
		msg = strings.Join(msgs, "; ")
	}
	return component.Health{Name: s.name, Status: worst, Message: msg}
}

// Describe returns summary information for the bootstrap startup display.
func (s *Scheduler) Describe() component.Description {
	names := make([]string, 0, len(s.workers))
	for _, w := range s.workers {
		names = append(names, w.Name())
	}
	return component.Description{
		Name:    s.name,
		Type:    "scheduler",
		Details: fmt.Sprintf("%d jobs: %s", len(s.workers), strings.Join(names, ", ")),
	}
}

// Workers returns the internal TickerWorkers for inspection (e.g. in tests).
func (s *Scheduler) Workers() []*TickerWorker { return s.workers }

func statusSeverity(s component.HealthStatus) int {
	switch s {
	case component.StatusHealthy:
		return 0
	case component.StatusDegraded:
		return 1
	case component.StatusUnhealthy:
		return 2
	default:
		return 3
	}
}
